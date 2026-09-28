package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestConnectionsShowAsWarmPathsAtTheirCompany(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	imported, err := hub.ImportConnections(ctx, owner, []store.NewConnection{
		{FirstName: "Ada", LastName: "Lovelace", ProfileURL: "https://www.linkedin.com/in/ada-example", CompanyName: "Acme, Inc.", Position: "Engineering Manager"},
		{FirstName: "Grace", LastName: "Hopper", ProfileURL: "https://www.linkedin.com/in/grace-example", CompanyName: "Globex", Position: "Engineer"},
	})
	if err != nil || imported.Matched != 0 {
		t.Fatalf("import = %+v, %v; no company is in the hub yet", imported, err)
	}

	acme, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	dossier, _ := hub.GetCompanyDossier(ctx, acme.ID)
	if len(dossier.Connections) != 1 || dossier.Connections[0].FirstName != "Ada" {
		t.Fatalf("dossier connections = %+v; a company added after the import picks up its connections", dossier.Connections)
	}
	summaries, _ := hub.ListCompanySummaries(ctx)
	if len(summaries) != 1 || summaries[0].ConnectionCount != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(48 * time.Hour)
	if _, err := hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, []store.JobPosting{{
		ExternalID: "9", CompanyName: "Globex", Title: "Frontend Engineer", Location: "Worldwide",
		URL: "https://himalayas.app/companies/globex/jobs/9", ExpiresAt: &expiresAt,
	}}, now); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	details, err := hub.GetJobDetails(ctx, jobs[0].Job.ID)
	if err != nil || len(details.Connections) != 1 || details.Connections[0].FirstName != "Grace" {
		t.Fatalf("details connections = %+v, %v; a feed job's company name finds its connections", details.Connections, err)
	}
}

func TestAFeedJobIsTiedToItsCompanyWhenTheCompanyArrives(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	now := time.Now().UTC()
	expiresAt := now.Add(48 * time.Hour)
	posting := store.JobPosting{
		ExternalID: "1", CompanyName: "Globex Inc.", Title: "Frontend Engineer", Location: "Worldwide",
		URL: "https://himalayas.app/companies/globex/jobs/1", ExpiresAt: &expiresAt,
	}
	if _, err := hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, []store.JobPosting{posting}, now); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	card, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &jobs[0].Job.ID})

	globex, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Globex", Domain: "globex.com"})
	if err != nil {
		t.Fatal(err)
	}
	jobs, _, _ = hub.ListJobs(ctx, store.JobFilter{CompanyID: &globex.ID})
	if len(jobs) != 1 {
		t.Fatalf("Globex's jobs = %+v; the feed job should now be Globex's", jobs)
	}
	if found, _ := hub.FindCompanyApplication(ctx, globex.ID); found.ID != card.ID {
		t.Fatalf("card = %+v; the job's card follows it to Globex", found)
	}
}

func TestConnectionsAreListedClosestFirstWithWhy(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.ImportConnections(ctx, owner, []store.NewConnection{
		{FirstName: "Old", LastName: "Acquaintance", ProfileURL: "https://www.linkedin.com/in/old", CompanyName: "Acme", ConnectedOn: date(2013, 4, 1)},
		{FirstName: "Recent", LastName: "Colleague", ProfileURL: "https://www.linkedin.com/in/recent", CompanyName: "Acme", ConnectedOn: date(2022, 1, 1)},
		{FirstName: "Long", LastName: "Ago", ProfileURL: "https://www.linkedin.com/in/long-ago", CompanyName: "Acme", ConnectedOn: date(2014, 1, 1)},
	}); err != nil {
		t.Fatal(err)
	}
	ownerURL := "https://www.linkedin.com/in/owner-example"
	message := func(conversation, sender string, at time.Time) store.NewLinkedInMessage {
		recipient := ownerURL
		if sender == ownerURL {
			recipient = "https://www.linkedin.com/in/recent"
		}
		return store.NewLinkedInMessage{ConversationID: conversation, SenderProfileURL: sender, RecipientProfileURLs: []string{recipient}, SentAt: at}
	}
	recently := time.Now().AddDate(0, -2, 0)
	if _, err := hub.ImportLinkedInMessages(ctx, owner, []store.NewLinkedInMessage{
		message("c1", "https://www.linkedin.com/in/recent", recently),
		message("c1", ownerURL, recently.Add(time.Hour)),
		{ConversationID: "c2", SenderProfileURL: "https://www.linkedin.com/in/long-ago", RecipientProfileURLs: []string{ownerURL}, SentAt: time.Date(2016, 5, 1, 0, 0, 0, 0, time.UTC)},
		{ConversationID: "c3", SenderProfileURL: ownerURL, RecipientProfileURLs: []string{"https://www.linkedin.com/in/someone"}, SentAt: recently},
	}); err != nil {
		t.Fatal(err)
	}
	acme, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})

	connections, err := hub.ListCompanyConnections(ctx, acme.ID)
	if err != nil || len(connections) != 3 {
		t.Fatalf("connections = %+v, %v", connections, err)
	}
	order := []string{connections[0].FirstName, connections[1].FirstName, connections[2].FirstName}
	if order[0] != "Recent" || order[1] != "Long" || order[2] != "Old" {
		t.Fatalf("order = %v; want the recent talk first, then the old talk, then never talked", order)
	}
	if connections[0].Closeness != "2 messages, last in "+recently.Add(time.Hour).UTC().Format("Jan 2006") || connections[2].Closeness != "connected in 2013, never talked" {
		t.Fatalf("closeness = %q, %q", connections[0].Closeness, connections[2].Closeness)
	}
}

func date(year int, month time.Month, day int) *time.Time {
	value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &value
}

func TestVouchesRaiseAConnectionAndSayWhy(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.ImportConnections(ctx, owner, []store.NewConnection{
		{FirstName: "Plain", LastName: "Contact", ProfileURL: "https://www.linkedin.com/in/plain", CompanyName: "Acme", ConnectedOn: date(2013, 1, 1)},
		{FirstName: "Ada", LastName: "Lovelace", ProfileURL: "https://www.linkedin.com/in/ada", CompanyName: "Acme", ConnectedOn: date(2015, 1, 1)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.ImportLinkedInEndorsements(ctx, owner, []store.NewLinkedInEndorsement{
		{Direction: store.VouchedReceived, Skill: "React", FirstName: "Ada", LastName: "Lovelace", ProfileURL: "https://www.linkedin.com/in/ada/", Status: "accepted"},
		{Direction: store.VouchedReceived, Skill: "Go", FirstName: "Ada", LastName: "Lovelace", ProfileURL: "https://www.linkedin.com/in/ada", Status: "rejected"},
	}); err != nil {
		t.Fatal(err)
	}
	imported, err := hub.ImportLinkedInRecommendations(ctx, owner, []store.NewLinkedInRecommendation{
		{Direction: store.VouchedReceived, FirstName: "Ada", LastName: "Lovelace", Company: "Acme", JobTitle: "CTO", Text: "Tony ships.", WrittenAt: date(2013, 1, 19), Status: "visible"},
	})
	if err != nil || imported.ConnectionsWithVouches != 1 {
		t.Fatalf("import = %+v, %v", imported, err)
	}
	acme, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})

	connections, _ := hub.ListCompanyConnections(ctx, acme.ID)
	if connections[0].FirstName != "Ada" || connections[0].Closeness != "connected in 2015, never talked; recommended you; endorsed you for React" {
		t.Fatalf("first = %q %q; want Ada first, vouched for React only, since the Go endorsement was rejected", connections[0].FirstName, connections[0].Closeness)
	}
	recommendations, _ := hub.ListRecommendationsReceived(ctx)
	if len(recommendations) != 1 || recommendations[0].Text != "Tony ships." {
		t.Fatalf("recommendations = %+v", recommendations)
	}
}
