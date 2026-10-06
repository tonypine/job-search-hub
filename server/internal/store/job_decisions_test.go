package store_test

import (
	"context"
	"errors"
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
	if leftForLater := listJobTitles(t, hub, store.JobStatusLater); len(leftForLater) != 1 || leftForLater[0] != "Later" {
		t.Errorf("later = %v, want the job left for later", leftForLater)
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

func TestClearingADecisionLeavesTheJobUndecided(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	later, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Later", URL: "https://acme.com/later"})
	skipped, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Skipped", URL: "https://acme.com/skipped"})
	undecided, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Undecided", URL: "https://acme.com/undecided"})
	if _, err := hub.DecideJob(ctx, owner, later.ID, store.JobDecisionLater, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.DecideJob(ctx, owner, skipped.ID, store.JobDecisionSkip, "agency"); err != nil {
		t.Fatal(err)
	}

	for _, id := range []uuid.UUID{later.ID, skipped.ID, undecided.ID} {
		if err := hub.ClearJobDecision(ctx, owner, id); err != nil {
			t.Fatal(err)
		}
		if decision, err := hub.GetJobDecision(ctx, id); err != nil || decision != nil {
			t.Errorf("after clearing, decision = %+v, %v; want none", decision, err)
		}
	}
	if open := listJobTitles(t, hub, store.JobStatusOpen); len(open) != 3 {
		t.Errorf("clearing a skip didn't restore the job: open = %v", open)
	}
	var cleared string
	if err := pool.QueryRow(ctx, `SELECT before->>'decision' FROM changes WHERE operation = 'undecide' AND entity_id = $1`, skipped.ID).Scan(&cleared); err != nil || cleared != store.JobDecisionSkip {
		t.Errorf("the undecide change's decision = %q, %v", cleared, err)
	}
	var undecideChanges int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM changes WHERE operation = 'undecide' AND entity_id = $1`, undecided.ID).Scan(&undecideChanges); err != nil || undecideChanges != 0 {
		t.Errorf("clearing an undecided job recorded %d changes, %v", undecideChanges, err)
	}
	if err := hub.ClearJobDecision(ctx, owner, uuid.New()); !errors.Is(err, store.ErrJobNotFound) {
		t.Errorf("an unknown job: %v, want ErrJobNotFound", err)
	}
}
