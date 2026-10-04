package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func alertPosting(externalID, title, description string) store.JobPosting {
	return store.JobPosting{
		ExternalID: externalID, CompanyName: "Acme Labs", Title: title, Location: "São Paulo, São Paulo", Description: description,
		URL: "https://www.glassdoor.com.br/job-listing/j?jl=" + externalID,
	}
}

func TestAlertJobsAwaitingTextGetTheirTextOrTheReasonTheyHaveNone(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	now := time.Now()
	fullText := strings.Repeat("The whole posting. ", 40)
	postings := []store.JobPosting{
		alertPosting("1", "Senior Frontend Engineer", ""), alertPosting("2", "Staff Frontend Engineer", "A snippet."),
		alertPosting("3", "Principal Frontend Engineer", fullText),
	}
	if _, _, err := hub.SyncAlertJobs(ctx, hubSystem, store.JobSourceGlassdoor, postings, now); err != nil {
		t.Fatal(err)
	}

	awaiting, err := hub.ListAlertJobsAwaitingText(ctx)
	if err != nil || len(awaiting) != 2 {
		t.Fatalf("awaiting = %+v, %v; want the two jobs without their text", awaiting, err)
	}
	ids := map[string]uuid.UUID{}
	for _, job := range awaiting {
		if job.CompanyName != "Acme Labs" || job.Searched {
			t.Errorf("awaiting = %+v", job)
		}
		ids[job.Job.Title] = job.Job.ID
	}

	if err := hub.SavePostingText(ctx, hubSystem, ids["Senior Frontend Engineer"], fullText, "https://acme.example/jobs/1", now); err != nil {
		t.Fatal(err)
	}
	reason := "the Glassdoor alert gave only a snippet, and neither a board found nor Google for Jobs lists the posting"
	if err := hub.RecordPostingTextMissing(ctx, ids["Staff Frontend Engineer"], reason, &now); err != nil {
		t.Fatal(err)
	}

	awaiting, err = hub.ListAlertJobsAwaitingText(ctx)
	if err != nil || len(awaiting) != 1 || !awaiting[0].Searched || awaiting[0].Job.TextMissingReason != reason {
		t.Fatalf("awaiting = %+v, %v; want only the job searched without finding its text, with its reason", awaiting, err)
	}
	var description string
	if err := pool.QueryRow(ctx, `SELECT description FROM jobs WHERE id = $1`, ids["Senior Frontend Engineer"]).Scan(&description); err != nil ||
		description != fullText {
		t.Errorf("found text = %q, %v", description, err)
	}
	if searches, err := hub.CountPostingTextSearchesSince(ctx, now.Add(-time.Minute)); err != nil || searches != 2 {
		t.Errorf("searches = %d, %v; want 2", searches, err)
	}
}

func TestABoardAdoptingAnAlertJobClearsTheReasonItHadNoText(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	now := time.Now()
	if _, _, err := hub.SyncAlertJobs(ctx, hubSystem, store.JobSourceGlassdoor, []store.JobPosting{alertPosting("1", "Senior Frontend Engineer", "")}, now); err != nil {
		t.Fatal(err)
	}
	awaiting, err := hub.ListAlertJobsAwaitingText(ctx)
	if err != nil || len(awaiting) != 1 {
		t.Fatalf("awaiting = %+v, %v", awaiting, err)
	}
	jobID := awaiting[0].Job.ID
	if err := hub.RecordPostingTextMissing(ctx, jobID, "the Glassdoor alert gave no text; looking for the posting on its company's board", nil); err != nil {
		t.Fatal(err)
	}
	board, err := hub.SaveFoundJobBoard(ctx, hubSystem, store.FoundJobBoardInput{CompanyName: "Acme Labs", Provider: "lever", BoardToken: "acmelabs"})
	if err != nil {
		t.Fatal(err)
	}

	if result, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{
		{ExternalID: "b1", Title: "Senior Frontend Engineer", URL: "https://jobs.lever.co/acmelabs/b1", Description: "The whole posting."},
	}, now); err != nil || result.Adopted != 1 {
		t.Fatalf("result = %+v, %v; want the alert job adopted", result, err)
	}

	details, err := hub.GetJobDetails(ctx, jobID)
	if err != nil || details.Job.TextMissingReason != "" || details.Job.Description != "The whole posting." {
		t.Errorf("adopted job = %+v, %v; want the board's text and no reason", details.Job, err)
	}
}

func TestAFailedSearchCountsAndIsNotAskedAgain(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	now := time.Now()
	if _, _, err := hub.SyncAlertJobs(ctx, hubSystem, store.JobSourceGlassdoor, []store.JobPosting{alertPosting("1", "Senior Frontend Engineer", "")}, now); err != nil {
		t.Fatal(err)
	}
	awaiting, err := hub.ListAlertJobsAwaitingText(ctx)
	if err != nil || len(awaiting) != 1 {
		t.Fatalf("awaiting = %+v, %v", awaiting, err)
	}

	reason := "the Glassdoor alert gave no text, no board found lists it, and the Google for Jobs search for it failed"
	if err := hub.RecordPostingTextSearchFailed(ctx, awaiting[0].Job.ID, reason, now); err != nil {
		t.Fatal(err)
	}

	awaiting, err = hub.ListAlertJobsAwaitingText(ctx)
	if err != nil || len(awaiting) != 1 || !awaiting[0].Searched || !awaiting[0].SearchFailed || awaiting[0].Job.TextMissingReason != reason {
		t.Fatalf("awaiting = %+v, %v; want the job searched, failed, with its reason", awaiting, err)
	}
	if searches, err := hub.CountPostingTextSearchesSince(ctx, now.Add(-time.Minute)); err != nil || searches != 1 {
		t.Errorf("searches = %d, %v; want the failed one counted", searches, err)
	}
}

func TestAnAlertRelistingAJobWithItsTextClearsTheReasonItHadNone(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	now := time.Now()
	if _, _, err := hub.SyncAlertJobs(ctx, hubSystem, store.JobSourceGlassdoor, []store.JobPosting{alertPosting("1", "Senior Frontend Engineer", "")}, now); err != nil {
		t.Fatal(err)
	}
	awaiting, err := hub.ListAlertJobsAwaitingText(ctx)
	if err != nil || len(awaiting) != 1 {
		t.Fatalf("awaiting = %+v, %v", awaiting, err)
	}
	jobID := awaiting[0].Job.ID
	if err := hub.RecordPostingTextMissing(ctx, jobID, "the Glassdoor alert gave no text; looking for the posting on its company's board", nil); err != nil {
		t.Fatal(err)
	}

	if _, _, err := hub.SyncAlertJobs(ctx, hubSystem, store.JobSourceGlassdoor,
		[]store.JobPosting{alertPosting("1", "Senior Frontend Engineer", "A snippet.")}, now); err != nil {
		t.Fatal(err)
	}
	details, err := hub.GetJobDetails(ctx, jobID)
	if err != nil || details.Job.TextMissingReason == "" {
		t.Fatalf("job = %+v, %v; a snippet should keep the reason", details.Job, err)
	}

	fullText := strings.Repeat("The whole posting. ", 40)
	if _, _, err := hub.SyncAlertJobs(ctx, hubSystem, store.JobSourceGlassdoor,
		[]store.JobPosting{alertPosting("1", "Senior Frontend Engineer", fullText)}, now); err != nil {
		t.Fatal(err)
	}
	details, err = hub.GetJobDetails(ctx, jobID)
	if err != nil || details.Job.TextMissingReason != "" || details.Job.Description != fullText {
		t.Errorf("job = %+v, %v; want the alert's text and no reason", details.Job, err)
	}
}

func TestAnAlertRelistingAJobWithASnippetKeepsTheTextFoundForIt(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	now := time.Now()
	if _, _, err := hub.SyncAlertJobs(ctx, hubSystem, store.JobSourceGlassdoor, []store.JobPosting{alertPosting("1", "Senior Frontend Engineer", "")}, now); err != nil {
		t.Fatal(err)
	}
	awaiting, err := hub.ListAlertJobsAwaitingText(ctx)
	if err != nil || len(awaiting) != 1 {
		t.Fatalf("awaiting = %+v, %v", awaiting, err)
	}
	jobID := awaiting[0].Job.ID
	fullText := strings.Repeat("The whole posting. ", 40)
	if err := hub.SavePostingText(ctx, hubSystem, jobID, fullText, "https://acme.example/jobs/1", now); err != nil {
		t.Fatal(err)
	}

	if _, _, err := hub.SyncAlertJobs(ctx, hubSystem, store.JobSourceGlassdoor,
		[]store.JobPosting{alertPosting("1", "Senior Frontend Engineer", "A snippet.")}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	details, err := hub.GetJobDetails(ctx, jobID)
	if err != nil || details.Job.Description != fullText || details.Job.TextMissingReason != "" {
		t.Errorf("job = %+v, %v; want the found text kept and no reason", details.Job, err)
	}
	if awaiting, err := hub.ListAlertJobsAwaitingText(ctx); err != nil || len(awaiting) != 0 {
		t.Errorf("awaiting = %+v, %v; want none", awaiting, err)
	}
}
