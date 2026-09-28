package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func phaseNames(phases []store.PipelinePhase) []string {
	names := make([]string, 0, len(phases))
	for _, phase := range phases {
		names = append(names, phase.Name)
	}
	return names
}

func equalNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func TestThePhasesAreSeededInOrder(t *testing.T) {
	phases, err := store.New(testdatabase.New(t)).ListPipelinePhases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Saved", "Applied", "In contact", "Interviewing", "Offer", "Closed"}; !equalNames(phaseNames(phases), want) {
		t.Fatalf("phases = %v, want %v", phaseNames(phases), want)
	}
	if !phases[5].IsClosed || phases[0].IsClosed {
		t.Fatal("only Closed should be a closed phase")
	}
}

func TestAJobGoesOnTheBoardOnceAndTakesItsCompany(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &company.ID, Title: "Engineer", URL: "https://acme.com/jobs/1"})
	if err != nil {
		t.Fatal(err)
	}

	first, created, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})
	if err != nil || !created || first.CompanyID == nil || *first.CompanyID != company.ID {
		t.Fatalf("add = %+v, created=%v, err=%v", first, created, err)
	}
	phases, _ := hub.ListPipelinePhases(ctx)
	if first.PhaseID != phases[0].ID {
		t.Fatal("a new application should start in the first phase")
	}
	again, created, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("second add = %+v, created=%v, err=%v", again, created, err)
	}

	missing := uuid.New()
	if _, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &missing}); !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("unknown job: err = %v", err)
	}
	cards, err := hub.ListPipelineCards(ctx)
	if err != nil || len(cards) != 1 || *cards[0].JobTitle != "Engineer" || *cards[0].CompanyName != "Acme" {
		t.Fatalf("cards = %+v, err = %v", cards, err)
	}
}

func TestMovingAnApplicationRecordsThePhaseItLeft(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	application, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID})
	phases, _ := hub.ListPipelinePhases(ctx)
	applied, closed := phases[1], phases[5]

	moved, err := hub.MoveApplication(ctx, owner, application.ID, applied.ID, "ignored outside Closed")
	if err != nil || moved.PhaseID != applied.ID || moved.ClosedReason != "" {
		t.Fatalf("move to Applied = %+v, %v", moved, err)
	}
	moved, err = hub.MoveApplication(ctx, owner, application.ID, closed.ID, "ghosted")
	if err != nil || moved.ClosedReason != "ghosted" {
		t.Fatalf("move to Closed = %+v, %v", moved, err)
	}

	var before, after []byte
	err = pool.QueryRow(ctx, `SELECT before, after FROM changes WHERE entity_type = 'application' AND operation = 'move' ORDER BY id DESC LIMIT 1`).Scan(&before, &after)
	if err != nil {
		t.Fatal(err)
	}
	var left, entered map[string]string
	json.Unmarshal(before, &left)
	json.Unmarshal(after, &entered)
	if left["phase_id"] != applied.ID.String() || entered["phase_id"] != closed.ID.String() || entered["closed_reason"] != "ghosted" {
		t.Fatalf("recorded move: before=%s after=%s", before, after)
	}
}

func TestPhasesCanBeRenamedReorderedAndDeletedOnlyWhenEmpty(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	phases, _ := hub.ListPipelinePhases(ctx)
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if _, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID}); err != nil {
		t.Fatal(err)
	}

	if err := hub.DeletePipelinePhase(ctx, owner, phases[0].ID); !errors.Is(err, store.ErrPipelinePhaseInUse) {
		t.Fatalf("deleting Saved with a card: err = %v", err)
	}
	if _, err := hub.RenamePipelinePhase(ctx, owner, phases[2].ID, "Talking"); err != nil {
		t.Fatal(err)
	}
	added, err := hub.AddPipelinePhase(ctx, owner, "Take-home", false)
	if err != nil || added.Position != 7 {
		t.Fatalf("add = %+v, %v", added, err)
	}
	reordered, err := hub.ReorderPipelinePhases(ctx, owner, []uuid.UUID{phases[0].ID, phases[1].ID, phases[2].ID, added.ID, phases[3].ID, phases[4].ID, phases[5].ID})
	want := []string{"Saved", "Applied", "Talking", "Take-home", "Interviewing", "Offer", "Closed"}
	if err != nil || !equalNames(phaseNames(reordered), want) {
		t.Fatalf("reordered = %v, %v; want %v", phaseNames(reordered), err, want)
	}
	if _, err := hub.ReorderPipelinePhases(ctx, owner, []uuid.UUID{phases[0].ID}); err == nil {
		t.Fatal("a partial order was accepted")
	}
	if err := hub.DeletePipelinePhase(ctx, owner, added.ID); err != nil {
		t.Fatalf("deleting the empty Take-home: %v", err)
	}
}

func TestAPhaseNameAlreadyInUseIsRefused(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	phases, err := hub.ListPipelinePhases(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := hub.AddPipelinePhase(ctx, owner, "saved", false); !errors.Is(err, store.ErrPipelinePhaseNameUsed) {
		t.Errorf("add a duplicate: err = %v, want ErrPipelinePhaseNameUsed", err)
	}
	if _, err := hub.RenamePipelinePhase(ctx, owner, phases[1].ID, "Saved"); !errors.Is(err, store.ErrPipelinePhaseNameUsed) {
		t.Errorf("rename to a duplicate: err = %v, want ErrPipelinePhaseNameUsed", err)
	}
}
