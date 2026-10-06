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
	postings := []store.JobPosting{posting("1", "Engineer"), posting("2", "Designer"), posting("3", "Untitled role")}
	postings[0].Description = "Build the API."
	postings[1].Description = "Design the app."
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, postings, time.Now()); err != nil {
		t.Fatal(err)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)

	if titles := awaitingTitles(t, hub, prompt); len(titles) != 2 {
		t.Fatalf("awaiting = %v, want the two jobs with a description", titles)
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

func TestAPostingWhoseTextChangesOnlyInItsMarkdownKeepsItsFacts(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	board := createBoard(t, hub)
	plain := posting("1", "Engineer")
	plain.Description = "About the role\nBuild & ship.\n\nYou have:\nReact\nTypeScript\nPay: $100k"
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{plain}, time.Now()); err != nil {
		t.Fatal(err)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	jobs, _ := hub.ListJobsAwaitingFacts(ctx, prompt.ID, 100)
	if len(jobs) != 1 {
		t.Fatalf("awaiting = %+v, want the engineer", jobs)
	}
	if err := hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: jobs[0].ID, PromptID: prompt.ID, Model: "test-model", TextHash: jobs[0].TextHash, Facts: json.RawMessage(`{"seniority":"Senior"}`)}); err != nil {
		t.Fatal(err)
	}

	// The next poll keeps the same words as Markdown.
	markdown := posting("1", "Engineer")
	markdown.Description = "### About the role\n\nBuild & ship.\n\nYou have:\n\n- React\n  1. TypeScript\n\n**Pay:** $100k"
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{markdown}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if titles := awaitingTitles(t, hub, prompt); len(titles) != 0 {
		t.Fatalf("awaiting after a change of markup alone = %v, want none", titles)
	}
	if count, _ := hub.CountJobsAwaitingFacts(ctx, prompt.ID); count != 0 {
		t.Fatalf("counted %d awaiting after a change of markup alone, want none", count)
	}

	reworded := posting("1", "Engineer")
	reworded.Description = "### About the role\n\nBuild & ship.\n\nYou have:\n\n- React\n- Go\n\n**Pay:** $100k"
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{reworded}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if titles := awaitingTitles(t, hub, prompt); len(titles) != 1 {
		t.Fatalf("awaiting after a change of words = %v, want the engineer", titles)
	}
}
