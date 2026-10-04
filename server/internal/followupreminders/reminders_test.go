package followupreminders

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func at(t *testing.T, moment string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", moment, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// fakeHub holds cards due at their DueAt, and drops a card from the list
// once it is reminded for that due time, as the store does.
type fakeHub struct {
	due       []store.DueFollowUp
	dueBefore time.Time
	reminded  map[uuid.UUID]time.Time
}

func (hub *fakeHub) ListFollowUpsToRemind(_ context.Context, dueBefore time.Time) ([]store.DueFollowUp, error) {
	hub.dueBefore = dueBefore
	var due []store.DueFollowUp
	for _, followUp := range hub.due {
		if remindedAt, reminded := hub.reminded[followUp.ApplicationID]; followUp.DueAt.Before(dueBefore) && (!reminded || remindedAt.Before(followUp.DueAt)) {
			due = append(due, followUp)
		}
	}
	return due, nil
}

func (hub *fakeHub) MarkFollowUpReminded(_ context.Context, id uuid.UUID, dueAt time.Time) error {
	hub.reminded[id] = dueAt
	return nil
}

type fakeUpdates struct {
	recorded []store.NewUpdate
	fail     bool
}

func (updates *fakeUpdates) Record(_ context.Context, input store.NewUpdate) (store.Update, error) {
	if updates.fail {
		return store.Update{}, errors.New("the database is gone")
	}
	updates.recorded = append(updates.recorded, input)
	return store.Update{Kind: input.Kind, Title: input.Title}, nil
}

func startReminder(t *testing.T, now string, due ...store.DueFollowUp) (*Reminder, *fakeHub, *fakeUpdates) {
	t.Helper()
	hub := &fakeHub{due: due, reminded: map[uuid.UUID]time.Time{}}
	updates := &fakeUpdates{}
	reminder := NewReminder(hub, updates)
	moment := at(t, now)
	reminder.now = func() time.Time { return moment }
	return reminder, hub, updates
}

func text(value string) *string { return &value }

func TestAFollowUpDueTodayIsToldOnceFromTheMorning(t *testing.T) {
	companyID, jobID := uuid.New(), uuid.New()
	applied := store.DueFollowUp{
		ApplicationID: uuid.New(), JobID: &jobID, CompanyID: &companyID, JobTitle: text("Senior Engineer"), CompanyName: text("Acme"),
		PhaseName: "Applied", PhaseEnteredAt: at(t, "2026-09-27 10:00"), DueAt: at(t, "2026-10-04 10:00"),
	}
	reminder, hub, updates := startReminder(t, "2026-10-04 07:59", applied)
	ctx := context.Background()

	if reminded, err := reminder.RemindOnce(ctx); err != nil || reminded != 0 || len(updates.recorded) != 0 {
		t.Fatalf("before the morning: reminded %d, %v, updates %v", reminded, err, updates.recorded)
	}

	moment := at(t, "2026-10-04 08:00")
	reminder.now = func() time.Time { return moment }
	if reminded, err := reminder.RemindOnce(ctx); err != nil || reminded != 1 {
		t.Fatalf("in the morning: reminded %d, %v", reminded, err)
	}
	if want := at(t, "2026-10-05 00:00"); !hub.dueBefore.Equal(want) {
		t.Fatalf("asked for follow-ups due before %v, want the end of the day %v", hub.dueBefore, want)
	}
	got := updates.recorded[0]
	if got.Kind != Kind || got.Title != "Follow up on Senior Engineer at Acme" || got.Body != "In Applied since Sep 27." ||
		got.JobID == nil || *got.JobID != jobID || got.CompanyID == nil || *got.CompanyID != companyID {
		t.Fatalf("update = %+v", got)
	}
	if remindedAt := hub.reminded[applied.ApplicationID]; !remindedAt.Equal(applied.DueAt) {
		t.Fatalf("reminded at %v, want the due time %v", remindedAt, applied.DueAt)
	}

	if reminded, err := reminder.RemindOnce(ctx); err != nil || reminded != 0 || len(updates.recorded) != 1 {
		t.Fatalf("a second pass: reminded %d, %v, updates %d", reminded, err, len(updates.recorded))
	}

	// A follow-up restarts the count; its next due time is told again.
	followedUpAt := moment
	hub.due[0].LastFollowedUpAt = &followedUpAt
	hub.due[0].DueAt = at(t, "2026-10-11 08:00")
	moment = at(t, "2026-10-11 09:00")
	if reminded, err := reminder.RemindOnce(ctx); err != nil || reminded != 1 || len(updates.recorded) != 2 {
		t.Fatalf("the next due time: reminded %d, %v, updates %d", reminded, err, len(updates.recorded))
	}
	if body := updates.recorded[1].Body; body != "In Applied since Sep 27. Last followed up Oct 4." {
		t.Fatalf("body = %q", body)
	}
}

func TestAnOutreachCardIsToldAsTheCompanyWithHowLateItIs(t *testing.T) {
	companyID := uuid.New()
	outreach := store.DueFollowUp{
		ApplicationID: uuid.New(), CompanyID: &companyID, CompanyName: text("Globex"),
		PhaseName: "Applied", PhaseEnteredAt: at(t, "2025-12-20 16:00"), DueAt: at(t, "2025-12-27 16:00"),
	}
	later := store.DueFollowUp{ApplicationID: uuid.New(), CompanyID: &companyID, PhaseName: "Applied", DueAt: at(t, "2026-01-03 09:00")}
	reminder, _, updates := startReminder(t, "2026-01-02 12:00", outreach, later)

	if reminded, err := reminder.RemindOnce(context.Background()); err != nil || reminded != 1 {
		t.Fatalf("reminded %d, %v; want only the card due by today", reminded, err)
	}
	got := updates.recorded[0]
	if got.Title != "Follow up with Globex" || got.Body != "In Applied since Dec 20, 2025. Due Dec 27, 2025." || got.JobID != nil {
		t.Fatalf("update = %+v", got)
	}
}

func TestAFailedUpdateLeavesTheCardToTellAgain(t *testing.T) {
	due := store.DueFollowUp{ApplicationID: uuid.New(), PhaseName: "Applied", DueAt: at(t, "2026-10-01 10:00")}
	reminder, hub, updates := startReminder(t, "2026-10-04 10:00", due)
	updates.fail = true

	if _, err := reminder.RemindOnce(context.Background()); err == nil {
		t.Fatal("a failed update reported no error")
	}
	if _, reminded := hub.reminded[due.ApplicationID]; reminded {
		t.Fatal("the card was marked reminded though no update was recorded")
	}
	updates.fail = false
	if reminded, err := reminder.RemindOnce(context.Background()); err != nil || reminded != 1 || updates.recorded[0].Title != "Follow up with the company" {
		t.Fatalf("the retry: reminded %d, %v, updates %+v", reminded, err, updates.recorded)
	}
}
