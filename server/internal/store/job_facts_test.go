package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func awaitingTitles(t *testing.T, hub *store.Store, prompt store.AgentPrompt) []string {
	t.Helper()
	jobs, err := hub.ListJobsAwaitingFacts(context.Background(), prompt.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, len(jobs))
	for _, job := range jobs {
		titles = append(titles, job.Title)
	}
	return titles
}

func TestJobsAwaitFactsUntilReadAndAgainWhenThePromptOrTheirTextChanges(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	board := createBoard(t, hub)
	postings := []store.JobPosting{posting("1", "Engineer"), posting("2", "Designer")}
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, postings, time.Now()); err != nil {
		t.Fatal(err)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)

	if titles := awaitingTitles(t, hub, prompt); len(titles) != 2 {
		t.Fatalf("awaiting = %v, want both jobs", titles)
	}
	jobs, _ := hub.ListJobsAwaitingFacts(ctx, prompt.ID, 100)
	for _, job := range jobs {
		if err := hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: job.ID, PromptID: prompt.ID, Model: "test-model", TextHash: job.TextHash, Facts: json.RawMessage(`{"seniority":"Senior"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	if titles := awaitingTitles(t, hub, prompt); len(titles) != 0 {
		t.Fatalf("awaiting after reading = %v, want none", titles)
	}

	// The engineer posting's description changes, and the designer's closes.
	changed := posting("1", "Engineer")
	changed.Description = "Now with Go."
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{changed}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if titles := awaitingTitles(t, hub, prompt); len(titles) != 1 || titles[0] != "Engineer" {
		t.Fatalf("awaiting after a change = %v, want only the engineer", titles)
	}

	newPrompt, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentPromptKindJobFacts, Body: "Record more."})
	if err != nil {
		t.Fatal(err)
	}
	if titles := awaitingTitles(t, hub, newPrompt); len(titles) != 1 {
		t.Fatalf("awaiting under a new prompt = %v, want the one open job", titles)
	}
}
