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

func phasesByName(t *testing.T, hub *store.Store) map[string]store.PipelinePhase {
	t.Helper()
	phases, err := hub.ListPipelinePhases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]store.PipelinePhase{}
	for _, phase := range phases {
		byName[phase.Name] = phase
	}
	return byName
}

// isAboutAt reports whether got is want, give or take the hour a daylight
// saving change moves a span of days in the database's time zone.
func isAboutAt(got, want time.Time) bool {
	return got.Sub(want).Abs() <= time.Hour
}

func TestADueFollowUpIsListedOncePerDueTime(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	phases := phasesByName(t, hub)
	acme, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	addCard := func(title, phase string, enteredAt time.Time) store.Application {
		t.Helper()
		job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &acme.ID, Title: title, URL: "https://acme.com/jobs/" + title})
		if err != nil {
			t.Fatal(err)
		}
		card, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})
		if err != nil {
			t.Fatal(err)
		}
		if card, err = hub.MoveApplicationAsOf(ctx, owner, card.ID, phases[phase].ID, "No answer.", enteredAt, ""); err != nil {
			t.Fatal(err)
		}
		return card
	}
	tenDaysAgo := time.Now().Add(-10 * 24 * time.Hour).Truncate(time.Second)
	overdue := addCard("Engineer", "Applied", tenDaysAgo)
	addCard("Saved", "Saved", tenDaysAgo)
	addCard("Closed", "Closed", tenDaysAgo)
	dismissed := addCard("Dismissed", "Applied", tenDaysAgo)
	if _, err := hub.DismissApplication(ctx, owner, dismissed.ID, ""); err != nil {
		t.Fatal(err)
	}

	due, err := hub.ListFollowUpsToRemind(ctx, time.Now())
	if err != nil || len(due) != 1 {
		t.Fatalf("due = %+v, %v; want only the open, undismissed Applied card", due, err)
	}
	got := due[0]
	wantDueAt := tenDaysAgo.Add(7 * 24 * time.Hour)
	if got.ApplicationID != overdue.ID || got.JobTitle == nil || *got.JobTitle != "Engineer" || got.CompanyName == nil || *got.CompanyName != "Acme" ||
		got.CompanyID == nil || *got.CompanyID != acme.ID || got.PhaseName != "Applied" || !isAboutAt(got.DueAt, wantDueAt) || !got.PhaseEnteredAt.Equal(tenDaysAgo) {
		t.Fatalf("due follow-up = %+v; want Engineer at Acme due %v", got, wantDueAt)
	}
	if due, _ := hub.ListFollowUpsToRemind(ctx, got.DueAt); len(due) != 0 {
		t.Fatalf("due before its due time = %+v; want none", due)
	}

	if err := hub.MarkFollowUpReminded(ctx, overdue.ID, got.DueAt); err != nil {
		t.Fatal(err)
	}
	if due, _ := hub.ListFollowUpsToRemind(ctx, time.Now()); len(due) != 0 {
		t.Fatalf("after the reminder, due = %+v; want none", due)
	}

	// A follow-up eight days ago moves the due time to yesterday, which is
	// told again.
	if _, err := hub.RecordFollowUpAsOf(ctx, owner, overdue.ID, "Pinged the recruiter.", time.Now().Add(-8*24*time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	if due, _ := hub.ListFollowUpsToRemind(ctx, time.Now()); len(due) != 1 || due[0].LastFollowedUpAt == nil || !due[0].DueAt.After(wantDueAt) {
		t.Fatalf("after a follow-up, due = %+v; want the card again at its new due time", due)
	}

	if err := hub.MarkFollowUpReminded(ctx, uuid.New(), time.Now()); !errors.Is(err, store.ErrApplicationNotFound) {
		t.Fatalf("reminding an unknown card = %v", err)
	}
}
