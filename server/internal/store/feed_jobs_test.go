package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func feedPosting(externalID, company string, expiresAt time.Time) store.JobPosting {
	return store.JobPosting{
		ExternalID: externalID, CompanyName: company, Title: "Frontend Engineer " + externalID, Location: "Worldwide",
		URL: "https://himalayas.app/companies/x/jobs/" + externalID, ExpiresAt: &expiresAt,
	}
}

func TestAFeedSyncStoresLinksAndClosesPostingsAtTheirExpiry(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	known, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	postings := []store.JobPosting{feedPosting("1", "ACME", now.Add(48*time.Hour)), feedPosting("2", "Globex", now.Add(time.Hour))}

	first, err := hub.SyncFeedJobs(ctx, hubSystem, store.JobSourceHimalayas, postings, now)
	if err != nil || first.Created != 2 {
		t.Fatalf("first sync = %+v, %v", first, err)
	}
	again, err := hub.SyncFeedJobs(ctx, hubSystem, store.JobSourceHimalayas, postings, now)
	if err != nil || again.Created != 0 || again.Closed != 0 {
		t.Fatalf("second sync = %+v, %v; want no change", again, err)
	}

	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	companies := map[string]store.JobListItem{}
	for _, item := range jobs {
		companies[item.Job.Title] = item
	}
	if linked := companies["Frontend Engineer 1"]; linked.Job.CompanyID == nil || *linked.Job.CompanyID != known.ID || *linked.CompanyName != "Acme" {
		t.Fatalf("the Acme posting = %+v", linked)
	}
	if unlinked := companies["Frontend Engineer 2"]; unlinked.Job.CompanyID != nil || unlinked.CompanyName == nil || *unlinked.CompanyName != "Globex" {
		t.Fatalf("the Globex posting = %+v", unlinked)
	}

	// Two hours later posting 2 has expired, even though no search returned it.
	later, err := hub.SyncFeedJobs(ctx, hubSystem, store.JobSourceHimalayas, nil, now.Add(2*time.Hour))
	if err != nil || later.Closed != 1 {
		t.Fatalf("later sync = %+v, %v; want posting 2 closed", later, err)
	}
	// Its expiry is extended and the search returns it again.
	extended := feedPosting("2", "Globex", now.Add(72*time.Hour))
	if reopened, err := hub.SyncFeedJobs(ctx, hubSystem, store.JobSourceHimalayas, []store.JobPosting{extended}, now.Add(3*time.Hour)); err != nil || reopened.Reopened != 1 {
		t.Fatalf("reopen = %+v, %v", reopened, err)
	}
	if operations := jobOperations(t, pool); len(operations) != 4 {
		t.Fatalf("job changes = %v, want create, create, close, reopen", operations)
	}
}
