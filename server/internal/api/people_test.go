package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestThePeopleListShowsEveryRelationWithWhatTheirCompanyHasOpen(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	globex, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Globex", Domain: "globex.com"})
	initech, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Initech", Domain: "initech.com"})

	if _, _, err := service.hub.AddPerson(ctx, owner, store.PersonInput{
		CompanyID: globex.ID, Name: "Morgan Example", RoleTitle: "CTO", Relevance: "founder", SourceURL: "https://globex.com/about",
	}); err != nil {
		t.Fatal(err)
	}
	connections := "First Name,Last Name,URL,Email Address,Company,Position,Connected On\n" +
		"Ada,Example,https://www.linkedin.com/in/ada-example,,Globex,Engineer,01 Jan 2020\n"
	if status, body := send(t, http.MethodPost, service.url+"/v1/connections/import", ownerToken, connections); status != http.StatusOK {
		t.Fatalf("connections: %d %s", status, body)
	}
	if status, body := send(t, http.MethodPost, service.url+"/v1/companies/"+initech.ID.String()+"/warm-paths", ownerToken,
		`{"name":"Sam Example","how_known":"Former manager"}`); status != http.StatusCreated {
		t.Fatalf("warm path: %d %s", status, body)
	}
	if _, err := service.hub.ImportLinkedInMessages(ctx, owner, []store.NewLinkedInMessage{{
		ConversationID: "c1", SenderName: "Rita Example", SenderProfileURL: "https://www.linkedin.com/in/rita-example",
		RecipientProfileURLs: []string{"https://www.linkedin.com/in/owner-example"}, SentAt: time.Now().Add(-72 * time.Hour), Content: "A role at Globex",
	}}); err != nil {
		t.Fatal(err)
	}
	awaiting, _ := service.hub.ListConversationsAwaitingClassification(ctx, 10)
	if err := service.hub.SaveConversationClassification(ctx, awaiting[0].ID, store.ConversationClassification{
		Class: "recruiter_outreach", ClassifiedBy: store.ClassifiedByModel, HiringCompany: "Globex", Role: "Engineer",
	}); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(48 * time.Hour)
	if _, err := service.hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, []store.JobPosting{{
		ExternalID: "1", CompanyName: "Globex", Title: "Frontend Engineer", Location: "Worldwide",
		URL: "https://himalayas.app/companies/globex/jobs/1", ExpiresAt: &expiresAt,
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}

	type person struct {
		Key         string  `json:"key"`
		Relation    string  `json:"relation"`
		Name        string  `json:"name"`
		Role        string  `json:"role"`
		CompanyID   string  `json:"company_id"`
		CompanyName string  `json:"company_name"`
		LastContact *string `json:"last_contact_at"`
		Answered    *bool   `json:"answered"`
		OpenJobs    int     `json:"open_jobs"`
	}
	var list struct {
		People []person `json:"people"`
	}
	status, body := send(t, http.MethodGet, service.url+"/v1/people", ownerToken, "")
	if err := json.Unmarshal(body, &list); status != http.StatusOK || err != nil || len(list.People) != 4 {
		t.Fatalf("people: %d %s", status, body)
	}
	byRelation := map[string]person{}
	for _, listed := range list.People {
		byRelation[listed.Relation] = listed
	}
	rita := byRelation[store.RelationRecruiter]
	if list.People[0].Relation != store.RelationRecruiter || rita.CompanyID != globex.ID.String() || rita.LastContact == nil ||
		rita.Answered == nil || *rita.Answered || rita.OpenJobs != 1 {
		t.Fatalf("Rita = %+v; want first, at Globex, unanswered and hiring now", rita)
	}
	if ada := byRelation[store.RelationConnection]; ada.Name != "Ada Example" || ada.Role != "Engineer" || ada.OpenJobs != 1 || ada.Answered != nil {
		t.Fatalf("Ada = %+v", ada)
	}
	if sam := byRelation[store.RelationIntroducer]; sam.CompanyName != "Initech" || sam.Role != "Former manager" || sam.OpenJobs != 0 {
		t.Fatalf("Sam = %+v; want at Initech, which has nothing open", sam)
	}
	if morgan := byRelation[store.RelationContact]; morgan.CompanyName != "Globex" || morgan.Role != "CTO" || morgan.Key == "" {
		t.Fatalf("Morgan = %+v", morgan)
	}

	status, body = send(t, http.MethodGet, service.url+"/v1/people?company_id="+initech.ID.String(), ownerToken, "")
	if err := json.Unmarshal(body, &list); status != http.StatusOK || err != nil || len(list.People) != 1 || list.People[0].Name != "Sam Example" {
		t.Fatalf("people at Initech: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/people?company_id=initech", ownerToken, ""); status != http.StatusBadRequest {
		t.Errorf("a company_id that isn't an id: %d, want 400", status)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/people", startTriage(t, service).Token, ""); status != http.StatusForbidden {
		t.Errorf("an agent's read: %d, want 403", status)
	}
}
