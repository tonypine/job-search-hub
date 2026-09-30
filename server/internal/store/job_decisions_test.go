package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestEachDecisionActsAndIsRecorded(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	addJob := func(title string) uuid.UUID {
		job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: title, URL: "https://acme.com/" + title})
		if err != nil {
			t.Fatal(err)
		}
		return job.ID
	}
	pursued, skipped, later := addJob("Pursued"), addJob("Skipped"), addJob("Later")

	if decision, err := hub.DecideJob(ctx, owner, pursued, store.JobDecisionPursue, ""); err != nil || decision.Decision != store.JobDecisionPursue {
		t.Fatalf("pursue = %+v, %v", decision, err)
	}
	if cards, _ := hub.ListPipelineCards(ctx); len(cards) != 1 || *cards[0].Application.JobID != pursued {
		t.Errorf("after pursuing, cards = %+v", cards)
	}
	if decision, err := hub.DecideJob(ctx, owner, skipped, store.JobDecisionSkip, " agency "); err != nil || decision.Reason != "agency" {
		t.Fatalf("skip = %+v, %v", decision, err)
	}
	if dismissed := listJobTitles(t, hub, store.JobStatusDismissed); len(dismissed) != 1 || dismissed[0] != "Skipped" {
		t.Errorf("dismissed = %v", dismissed)
	}
	if _, err := hub.DecideJob(ctx, owner, later, store.JobDecisionLater, ""); err != nil {
		t.Fatal(err)
	}
	if open := listJobTitles(t, hub, store.JobStatusOpen); len(open) != 2 {
		t.Errorf("open = %v, want the pursued and later jobs", open)
	}
	details, _ := hub.GetJobDetails(ctx, later)
	if details.Decision == nil || details.Decision.Decision != store.JobDecisionLater {
		t.Errorf("details decision = %+v", details.Decision)
	}

	if _, err := hub.DecideJob(ctx, owner, skipped, store.JobDecisionPursue, ""); err != nil {
		t.Fatal(err)
	}
	if open := listJobTitles(t, hub, store.JobStatusOpen); len(open) != 3 {
		t.Errorf("pursuing a skipped job didn't restore it: open = %v", open)
	}
	if _, err := hub.DecideJob(ctx, owner, uuid.New(), store.JobDecisionLater, ""); err == nil {
		t.Error("expected an error for an unknown job")
	}
	if _, err := hub.DecideJob(ctx, owner, later, "maybe", ""); err == nil {
		t.Error("expected an error for an unknown decision")
	}
}

func TestADismissalIsASkipAndARestoreTakesItBack(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/1"})
	card, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})

	if _, err := hub.DismissApplication(ctx, owner, card.ID, "stack"); err != nil {
		t.Fatal(err)
	}
	if decision, _ := hub.GetJobDecision(ctx, job.ID); decision == nil || decision.Decision != store.JobDecisionSkip || decision.Reason != "not a good fit: stack" {
		t.Fatalf("after dismissing the card, decision = %+v", decision)
	}
	if _, err := hub.RestoreJobs(ctx, owner, []uuid.UUID{job.ID}); err != nil {
		t.Fatal(err)
	}
	if decision, _ := hub.GetJobDecision(ctx, job.ID); decision != nil {
		t.Errorf("after restoring, decision = %+v, want none", decision)
	}
}
