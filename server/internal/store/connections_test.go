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
