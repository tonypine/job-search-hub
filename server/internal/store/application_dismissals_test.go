package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestADismissedJobCardLeavesTheBoardAndTheJobsListTogether(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	board := createBoard(t, hub)
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{posting("1", "Product Engineer")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	card, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &jobs[0].Job.ID})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := hub.DismissApplication(ctx, owner, card.ID, " the stack is dated "); err != nil {
		t.Fatal(err)
	}
	if onBoard, _ := hub.ListPipelineCards(ctx); len(onBoard) != 0 {
		t.Fatalf("board = %+v, want the card off it", onBoard)
	}
	dismissed, err := hub.ListDismissedPipelineCards(ctx)
	if err != nil || len(dismissed) != 1 {
		t.Fatalf("dismissed = %+v, %v", dismissed, err)
	}
	if dismissed[0].Application.PhaseID != card.PhaseID || dismissed[0].DismissalReason != "not a good fit: the stack is dated" || dismissed[0].DismissedAt == nil {
		t.Errorf("dismissed card = %+v, want its phase kept and the reason", dismissed[0])
	}
	if listed := listJobTitles(t, hub, store.JobStatusDismissed); len(listed) != 1 {
		t.Errorf("dismissed jobs = %v, want the card's job", listed)
	}

	if _, err := hub.RestoreJobs(ctx, owner, []uuid.UUID{jobs[0].Job.ID}); err != nil {
		t.Fatal(err)
	}
	if onBoard, _ := hub.ListPipelineCards(ctx); len(onBoard) != 1 || onBoard[0].Application.PhaseID != card.PhaseID {
		t.Fatalf("after restoring the job from the Jobs list, board = %+v", onBoard)
	}
}

func TestACompanyCardIsDismissedAndRestoredOnItsOwn(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	card, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := hub.DismissApplication(ctx, owner, card.ID, ""); err != nil {
		t.Fatal(err)
	}
	if dismissed, _ := hub.ListDismissedPipelineCards(ctx); len(dismissed) != 1 || dismissed[0].DismissalReason != "not a good fit" {
		t.Fatalf("dismissed = %+v", dismissed)
	}
	if _, err := hub.RestoreApplication(ctx, owner, card.ID); err != nil {
		t.Fatal(err)
	}
	if onBoard, _ := hub.ListPipelineCards(ctx); len(onBoard) != 1 || onBoard[0].DismissedAt != nil {
		t.Fatalf("board = %+v", onBoard)
	}
	if _, err := hub.DismissApplication(ctx, owner, uuid.New(), ""); err == nil {
		t.Error("expected an error for an unknown card")
	}
}
