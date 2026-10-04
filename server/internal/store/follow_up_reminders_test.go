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

func TestOutreachPutsTheCompanysCardInAppliedAndASecondMessageFollowsUp(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	phases := phasesByName(t, hub)
	acme, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &acme.ID, Title: "Engineer", URL: "https://acme.com/jobs/1"})
	jobCard, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})
	if err != nil {
		t.Fatal(err)
	}

	sentAt := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	card, created, err := hub.RecordOutreach(ctx, owner, acme.ID, "LinkedIn message to the engineering lead", sentAt)
	if err != nil || !created || card.JobID != nil || card.CompanyID == nil || *card.CompanyID != acme.ID ||
		card.PhaseID != phases["Applied"].ID || !card.PhaseEnteredAt.Equal(sentAt) {
		t.Fatalf("outreach = %+v, created=%v, %v; want a new outreach card in Applied since %v", card, created, err, sentAt)
	}
	cards, _ := hub.ListPipelineCards(ctx)
	for _, listed := range cards {
		switch listed.Application.ID {
		case card.ID:
			if listed.FollowUpDueAt == nil || !isAboutAt(*listed.FollowUpDueAt, sentAt.Add(7*24*time.Hour)) {
				t.Fatalf("the outreach card is due %v; want a week after the message", listed.FollowUpDueAt)
			}
		case jobCard.ID:
			if listed.Application.PhaseID != phases["Saved"].ID {
				t.Fatal("outreach moved the company's job card")
			}
		}
	}
	var notes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM changes WHERE operation = 'outreach' AND entity_id = $1
		AND after->>'note' = 'LinkedIn message to the engineering lead'`, card.ID).Scan(&notes); err != nil || notes != 1 {
		t.Fatalf("outreach changes = %d, %v", notes, err)
	}

	again, created, err := hub.RecordOutreach(ctx, owner, acme.ID, "Emailed the recruiter", time.Now())
	if err != nil || created || again.ID != card.ID || again.PhaseID != phases["Applied"].ID || again.LastFollowedUpAt == nil ||
		!again.PhaseEnteredAt.Equal(sentAt) {
		t.Fatalf("a second message = %+v, created=%v, %v; want a follow-up on the same card", again, created, err)
	}

	// A company's card waiting in Saved moves; once closed, the next message
	// starts a new card.
	globex, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Globex", Domain: "globex.com"})
	saved, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &globex.ID})
	moved, created, err := hub.RecordOutreach(ctx, owner, globex.ID, "", time.Now())
	if err != nil || created || moved.ID != saved.ID || moved.PhaseID != phases["Applied"].ID {
		t.Fatalf("outreach to a company in Saved = %+v, created=%v, %v; want its card moved", moved, created, err)
	}
	if _, err := hub.MoveApplication(ctx, owner, moved.ID, phases["Closed"].ID, "No answer."); err != nil {
		t.Fatal(err)
	}
	fresh, created, err := hub.RecordOutreach(ctx, owner, globex.ID, "", time.Now())
	if err != nil || !created || fresh.ID == moved.ID {
		t.Fatalf("outreach after the card closed = %+v, created=%v, %v; want a new card", fresh, created, err)
	}

	if _, _, err := hub.RecordOutreach(ctx, owner, uuid.New(), "", time.Now()); !errors.Is(err, store.ErrCompanyNotFound) {
		t.Fatalf("outreach to an unknown company = %v", err)
	}
	if _, err := hub.RenamePipelinePhase(ctx, owner, phases["Applied"].ID, "Reached out"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := hub.RecordOutreach(ctx, owner, acme.ID, "", time.Now()); !errors.Is(err, store.ErrPipelinePhaseNotFound) {
		t.Fatalf("outreach without an Applied phase = %v", err)
	}
}
