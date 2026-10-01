package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestAFixIsLoggedWithItsReasonsAndOutlivesTheNextPoll(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	expiresAt := now.Add(48 * time.Hour)
	posting := store.JobPosting{
		ExternalID: "7", CompanyName: "Join our winning team!", Title: "Frontend Engineer - Track&Field - São Paulo", Location: "São Paulo",
		URL: "https://indeed.example/7", ExpiresAt: &expiresAt,
	}
	if _, err := hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, []store.JobPosting{posting}, now); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	jobID := jobs[0].Job.ID
	agent := store.Actor{Kind: store.ActorSystem}

	title, companyName := "Frontend Engineer", "Track&Field"
	for name, fix := range map[string]store.JobDetailsFix{
		"no reason":    {Title: &title},
		"stray reason": {Title: &title, Reasons: map[string]string{"title": "x", "location": "y"}},
		"empty title":  {Title: new(string), Reasons: map[string]string{"title": "x"}},
		"no field":     {},
	} {
		if _, err := hub.FixJobDetails(ctx, agent, jobID, fix); err == nil {
			t.Errorf("%s: the fix was applied", name)
		}
	}
	if _, err := hub.FixJobDetails(ctx, agent, uuid.New(), store.JobDetailsFix{Title: &title, Reasons: map[string]string{"title": "x"}}); !errors.Is(err, store.ErrJobNotFound) {
		t.Errorf("an unknown job: %v", err)
	}

	fixed, err := hub.FixJobDetails(ctx, agent, jobID, store.JobDetailsFix{
		Title: &title, CompanyName: &companyName,
		Reasons: map[string]string{"title": " the title carries the company and city ", "company_name": "the company field holds a slogan"},
	})
	if err != nil || fixed.Title != title {
		t.Fatalf("fixed = %+v, %v", fixed, err)
	}
	var fixes int
	var titleReason string
	if err := pool.QueryRow(ctx, `SELECT count(*), max(after->>'reason') FILTER (WHERE after ? 'title') FROM changes WHERE entity_id = $1 AND operation = 'fix'`, jobID).
		Scan(&fixes, &titleReason); err != nil || fixes != 2 || titleReason != "the title carries the company and city" {
		t.Errorf("fix changes = %d, title reason %q, %v", fixes, titleReason, err)
	}

	posting.Location = "São Paulo, Brazil"
	if _, err := hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, []store.JobPosting{posting}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var afterPoll struct{ title, companyName, location string }
	var fixedFields []string
	if err := pool.QueryRow(ctx, `SELECT title, company_name, location, fixed_fields FROM jobs WHERE id = $1`, jobID).
		Scan(&afterPoll.title, &afterPoll.companyName, &afterPoll.location, &fixedFields); err != nil {
		t.Fatal(err)
	}
	if afterPoll.title != title || afterPoll.companyName != companyName {
		t.Errorf("after the poll: %+v, want the fixed title and company kept", afterPoll)
	}
	if afterPoll.location != "São Paulo, Brazil" {
		t.Errorf("after the poll the location is %q, want the polled one", afterPoll.location)
	}
	if len(fixedFields) != 2 || fixedFields[0] != "company_name" || fixedFields[1] != "title" {
		t.Errorf("fixed fields = %v", fixedFields)
	}
}

func TestABoardsJobKeepsAFixAndItsBoardsCompany(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	board := createBoard(t, hub)
	posting := store.JobPosting{ExternalID: "1", Title: "Engineer (Remote, LATAM)", Location: "Remote", URL: "https://acme.com/jobs/1"}
	if _, err := hub.SyncBoardJobs(ctx, owner, board, []store.JobPosting{posting}, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	title := "Engineer"
	if _, err := hub.FixJobDetails(ctx, owner, jobs[0].Job.ID, store.JobDetailsFix{Title: &title, Reasons: map[string]string{"title": "the place is in the title"}}); err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	if _, err := hub.FixJobDetails(ctx, owner, jobs[0].Job.ID, store.JobDetailsFix{CompanyID: &other, Reasons: map[string]string{"company_id": "x"}}); !errors.Is(err, store.ErrJobCompanyFromBoard) {
		t.Errorf("tying a board's job elsewhere: %v", err)
	}

	posting.Location = "Remote, LATAM"
	if _, err := hub.SyncBoardJobs(ctx, owner, board, []store.JobPosting{posting}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if after, _, _ := hub.ListJobs(ctx, store.JobFilter{}); after[0].Job.Title != title || after[0].Job.Location != "Remote, LATAM" {
		t.Errorf("after the board's poll: %q at %q", after[0].Job.Title, after[0].Job.Location)
	}
}
