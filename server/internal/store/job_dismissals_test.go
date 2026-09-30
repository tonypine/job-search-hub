package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func listJobTitles(t *testing.T, hub *store.Store, status string) []string {
	t.Helper()
	items, total, err := hub.ListJobs(context.Background(), store.JobFilter{Status: status})
	if err != nil {
		t.Fatal(err)
	}
	titles := []string{}
	for _, item := range items {
		titles = append(titles, item.Job.Title)
	}
	slices.Sort(titles)
	if total != len(titles) {
		t.Fatalf("%s: total %d for %d jobs", status, total, len(titles))
	}
	return titles
}

func TestADismissedJobLeavesTheListAndStaysDismissedWhenListedAgain(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	board := createBoard(t, hub)
	seenAt := time.Now()
	postings := []store.JobPosting{posting("1", "Agency Role"), posting("2", "Product Engineer"), posting("3", "Old Role")}
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, postings, seenAt); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, postings[:2], seenAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, _, err := hub.ListJobs(ctx, store.JobFilter{Status: store.JobStatusAll})
	if err != nil {
		t.Fatal(err)
	}
	idOf := map[string]uuid.UUID{}
	for _, item := range items {
		idOf[item.Job.Title] = item.Job.ID
	}

	dismissed, err := hub.DismissJobs(ctx, owner, []uuid.UUID{idOf["Agency Role"], idOf["Old Role"], idOf["Agency Role"]}, "agency")
	if err != nil {
		t.Fatal(err)
	}
	if len(dismissed) != 2 || dismissed[0].DismissedAt == nil || dismissed[0].DismissalReason != "agency" {
		t.Fatalf("dismissed = %+v", dismissed)
	}
	if open := listJobTitles(t, hub, store.JobStatusOpen); !slices.Equal(open, []string{"Product Engineer"}) {
		t.Errorf("open = %v", open)
	}
	if closed := listJobTitles(t, hub, store.JobStatusClosed); len(closed) != 0 {
		t.Errorf("closed = %v, want the dismissed closed job left out", closed)
	}
	if all := listJobTitles(t, hub, store.JobStatusAll); !slices.Equal(all, []string{"Product Engineer"}) {
		t.Errorf("all = %v", all)
	}
	if listed := listJobTitles(t, hub, store.JobStatusDismissed); !slices.Equal(listed, []string{"Agency Role", "Old Role"}) {
		t.Errorf("dismissed = %v", listed)
	}

	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, postings, seenAt.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if open := listJobTitles(t, hub, store.JobStatusOpen); !slices.Equal(open, []string{"Product Engineer"}) {
		t.Errorf("after the board lists them again, open = %v", open)
	}

	restored, err := hub.RestoreJobs(ctx, owner, []uuid.UUID{idOf["Agency Role"]})
	if err != nil || len(restored) != 1 || restored[0].DismissedAt != nil || restored[0].DismissalReason != "" {
		t.Fatalf("restored = %+v, %v", restored, err)
	}
	if open := listJobTitles(t, hub, store.JobStatusOpen); !slices.Equal(open, []string{"Agency Role", "Product Engineer"}) {
		t.Errorf("after restoring, open = %v", open)
	}
	operations := jobOperations(t, pool)
	if !slices.Contains(operations, "dismiss:owner") || !slices.Contains(operations, "restore:owner") {
		t.Errorf("operations = %v", operations)
	}
	var reason string
	if err := pool.QueryRow(ctx, `SELECT after->>'reason' FROM changes WHERE operation = 'dismiss' AND entity_id = $1`, idOf["Old Role"]).Scan(&reason); err != nil || reason != "agency" {
		t.Errorf("the change log's reason = %q, %v", reason, err)
	}
}

func TestDismissingAnUnknownJobDismissesNothing(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	board := createBoard(t, hub)
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{posting("1", "Product Engineer")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	items, _, _ := hub.ListJobs(ctx, store.JobFilter{})

	if _, err := hub.DismissJobs(ctx, owner, []uuid.UUID{items[0].Job.ID, uuid.New()}, ""); !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("error = %v, want ErrJobNotFound", err)
	}
	if open := listJobTitles(t, hub, store.JobStatusOpen); len(open) != 1 {
		t.Errorf("open = %v, want the known job left alone", open)
	}
	if _, err := hub.DismissJobs(ctx, owner, nil, ""); err == nil {
		t.Error("expected an error for no jobs")
	}
}

func TestADismissedJobAwaitsNoFacts(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	board := createBoard(t, hub)
	withText := posting("1", "Product Engineer")
	withText.Description = "Build the product."
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{withText}, time.Now()); err != nil {
		t.Fatal(err)
	}
	items, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	prompt, err := hub.GetLatestAgentPrompt(ctx, "job_facts")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := hub.CountJobsAwaitingFacts(ctx, prompt.ID)
	if _, err := hub.DismissJobs(ctx, owner, []uuid.UUID{items[0].Job.ID}, ""); err != nil {
		t.Fatal(err)
	}
	after, _ := hub.CountJobsAwaitingFacts(ctx, prompt.ID)
	awaiting, _ := hub.ListJobsAwaitingFacts(ctx, prompt.ID, 10)
	if before != 1 || after != 0 || len(awaiting) != 0 {
		t.Fatalf("awaiting facts: %d before, %d after, %d listed", before, after, len(awaiting))
	}
}
