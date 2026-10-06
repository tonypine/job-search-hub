package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestThePeopleListHasEveryRelationAtItsCompany(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	acme, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	globex, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Globex", Domain: "globex.com"})

	if _, _, err := hub.AddPerson(ctx, owner, store.PersonInput{
		CompanyID: acme.ID, Name: "Morgan Example", RoleTitle: "Head of Engineering", Relevance: "hiring_manager",
		SourceURL: "https://acme.com/team",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.ImportConnections(ctx, owner, []store.NewConnection{
		{FirstName: "Ada", LastName: "Example", ProfileURL: "https://www.linkedin.com/in/ada-example", CompanyName: "Acme, Inc.", Position: "Engineering Manager"},
		{FirstName: "Zed", LastName: "Example", ProfileURL: "https://www.linkedin.com/in/zed-example", CompanyName: "Elsewhere", Position: "Engineer"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, company := range []uuid.UUID{acme.ID, globex.ID} {
		if _, err := hub.AddWarmPath(ctx, owner, company, store.NewWarmPath{Name: "Sam Example", HowKnown: "Former colleague", Note: "Interviewed there"}); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	ownerURL := "https://www.linkedin.com/in/owner-example"
	if _, err := hub.ImportLinkedInMessages(ctx, owner, []store.NewLinkedInMessage{
		{ConversationID: "c1", SenderName: "Rita Example", SenderProfileURL: "https://www.linkedin.com/in/rita-example",
			RecipientProfileURLs: []string{ownerURL}, SentAt: at, Content: "A role at Acme"},
		{ConversationID: "c2", SenderName: "Hal Example", SenderProfileURL: "https://www.linkedin.com/in/hal-example",
			RecipientProfileURLs: []string{ownerURL}, SentAt: at.Add(-24 * time.Hour), Content: "A role somewhere"},
		{ConversationID: "c2", SenderName: "Owner", SenderProfileURL: ownerURL,
			RecipientProfileURLs: []string{"https://www.linkedin.com/in/hal-example"}, SentAt: at.Add(-23 * time.Hour), Content: "Thanks"},
	}); err != nil {
		t.Fatal(err)
	}
	awaiting, _ := hub.ListConversationsAwaitingClassification(ctx, 10)
	for _, conversation := range awaiting {
		classification := store.ConversationClassification{Class: "recruiter_outreach", ClassifiedBy: store.ClassifiedByModel, Role: "Engineer"}
		if conversation.StartedByName == "Rita Example" {
			classification.HiringCompany = "Acme Inc."
		} else {
			classification.IsAgency = true
		}
		if err := hub.SaveConversationClassification(ctx, conversation.ID, classification); err != nil {
			t.Fatal(err)
		}
	}

	people, err := hub.ListRelatedPeople(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	type shown struct {
		relation, name, company string
	}
	var got []shown
	for _, person := range people {
		got = append(got, shown{person.Relation, person.Name, person.CompanyName})
	}
	want := []shown{
		{store.RelationRecruiter, "Rita Example", "Acme"},
		{store.RelationRecruiter, "Hal Example", ""},
		{store.RelationIntroducer, "Sam Example", "Acme"},
		{store.RelationIntroducer, "Sam Example", "Globex"},
		{store.RelationConnection, "Ada Example", "Acme"},
		{store.RelationContact, "Morgan Example", "Acme"},
	}
	if len(got) != len(want) {
		t.Fatalf("people = %+v; want %+v (a connection at no hub company is left out)", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("people = %+v; want %+v, the latest contacted first, then by relation", got, want)
		}
	}

	rita, hal := people[0], people[1]
	if rita.CompanyID == nil || *rita.CompanyID != acme.ID || rita.Answered == nil || *rita.Answered || rita.LastContactAt == nil ||
		!rita.LastContactAt.Equal(at) || rita.HiringRole != "Engineer" || rita.Key != "recruiter:"+rita.ID.String() {
		t.Fatalf("Rita = %+v; want at Acme, unanswered, last written to on %v", rita, at)
	}
	if hal.CompanyID != nil || hal.Answered == nil || !*hal.Answered || !hal.IsAgency {
		t.Fatalf("Hal = %+v; want an answered agency recruiter at no hub company", hal)
	}
	if people[2].Key == people[3].Key || people[2].Role != "Former colleague" || people[2].Note != "Interviewed there" || people[2].Answered != nil {
		t.Fatalf("introducers = %+v, %+v; want one per company, each with how they're known", people[2], people[3])
	}
	if ada := people[4]; ada.Role != "Engineering Manager" || ada.Closeness == "" || ada.ProfileURL == "" {
		t.Fatalf("Ada = %+v; want their position, closeness and profile", ada)
	}
	if morgan := people[5]; morgan.Role != "Head of Engineering" || morgan.Relevance != "hiring_manager" || morgan.SourceURL != "https://acme.com/team" {
		t.Fatalf("Morgan = %+v; want their role, relevance and source", morgan)
	}

	atGlobex, err := hub.ListRelatedPeople(ctx, &globex.ID)
	if err != nil || len(atGlobex) != 1 || atGlobex[0].Relation != store.RelationIntroducer {
		t.Fatalf("at Globex = %+v, %v; want Sam alone", atGlobex, err)
	}
	atAcme, _ := hub.ListRelatedPeople(ctx, &acme.ID)
	if len(atAcme) != 4 {
		t.Fatalf("at Acme = %+v; want Rita, Sam, Ada and Morgan", atAcme)
	}
	nowhere := uuid.New()
	if none, err := hub.ListRelatedPeople(ctx, &nowhere); err != nil || len(none) != 0 || none == nil {
		t.Fatalf("at an unknown company = %+v, %v; want an empty list", none, err)
	}
}
