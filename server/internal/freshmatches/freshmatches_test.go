package freshmatches

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

// fakeHub holds strong matches, and drops one from the list once it's
// posted before postedAfter or told, as the store does.
type fakeHub struct {
	matches     []store.FreshMatch
	postedAfter time.Time
	told        map[uuid.UUID]time.Time
}

func (hub *fakeHub) ListFreshMatchesToTell(_ context.Context, postedAfter time.Time) ([]store.FreshMatch, error) {
	hub.postedAfter = postedAfter
	var fresh []store.FreshMatch
	for _, match := range hub.matches {
		posted := match.FirstSeenAt
		if match.PublishedAt != nil {
			posted = *match.PublishedAt
		}
		if _, told := hub.told[match.JobID]; !told && posted.After(postedAfter) {
			fresh = append(fresh, match)
		}
	}
	return fresh, nil
}

func (hub *fakeHub) MarkFreshMatchTold(_ context.Context, jobID uuid.UUID, toldAt time.Time) error {
	hub.told[jobID] = toldAt
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

func startTeller(t *testing.T, now string, matches ...store.FreshMatch) (*Teller, *fakeHub, *fakeUpdates) {
	t.Helper()
	hub := &fakeHub{matches: matches, told: map[uuid.UUID]time.Time{}}
	updates := &fakeUpdates{}
	teller := NewTeller(hub, updates)
	moment := at(t, now)
	teller.now = func() time.Time { return moment }
	return teller, hub, updates
}

func text(value string) *string { return &value }

func moment(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed := at(t, value)
	return &parsed
}

func TestAFreshStrongMatchIsToldOnce(t *testing.T) {
	companyID := uuid.New()
	match := store.FreshMatch{
		JobID: uuid.New(), CompanyID: &companyID, JobTitle: "Senior Engineer", CompanyName: text("Acme"),
		PublishedAt: moment(t, "2026-10-04 05:00"), FirstSeenAt: at(t, "2026-10-04 09:30"), Reason: "Go and Postgres, remote in the owner's region.",
	}
	teller, hub, updates := startTeller(t, "2026-10-04 10:00", match)
	ctx := context.Background()

	told, err := teller.TellOnce(ctx)
	if err != nil || told != 1 {
		t.Fatalf("told %d, error %v", told, err)
	}
	if want := at(t, "2026-10-01 10:00"); !hub.postedAfter.Equal(want) {
		t.Errorf("asked for matches posted after %v, want %v", hub.postedAfter, want)
	}
	if len(updates.recorded) != 1 {
		t.Fatalf("recorded %d updates", len(updates.recorded))
	}
	update := updates.recorded[0]
	if update.Kind != Kind || update.Title != "Strong match: Senior Engineer at Acme" {
		t.Errorf("update = %q %q", update.Kind, update.Title)
	}
	if want := "Posted 5 hours ago. Go and Postgres, remote in the owner's region."; update.Body != want {
		t.Errorf("body = %q, want %q", update.Body, want)
	}
	if update.JobID == nil || *update.JobID != match.JobID || update.CompanyID == nil || *update.CompanyID != companyID {
		t.Errorf("the update isn't about the job and its company: %+v", update)
	}
	if toldAt := hub.told[match.JobID]; !toldAt.Equal(at(t, "2026-10-04 10:00")) {
		t.Errorf("told at %v", toldAt)
	}

	if told, err := teller.TellOnce(ctx); err != nil || told != 0 {
		t.Errorf("a second pass told %d, error %v", told, err)
	}
	if len(updates.recorded) != 1 {
		t.Errorf("recorded %d updates in all, want 1", len(updates.recorded))
	}
}

func TestNothingIsToldInTheNight(t *testing.T) {
	match := store.FreshMatch{JobID: uuid.New(), JobTitle: "Engineer", PublishedAt: moment(t, "2026-10-03 21:00"), FirstSeenAt: at(t, "2026-10-03 21:10")}
	for _, now := range []string{"2026-10-03 22:00", "2026-10-04 03:00", "2026-10-04 07:59"} {
		teller, _, updates := startTeller(t, now, match)
		if told, err := teller.TellOnce(context.Background()); err != nil || told != 0 || len(updates.recorded) != 0 {
			t.Errorf("at %s told %d, error %v", now, told, err)
		}
	}
	teller, _, updates := startTeller(t, "2026-10-04 08:00", match)
	if told, err := teller.TellOnce(context.Background()); err != nil || told != 1 {
		t.Fatalf("in the morning told %d, error %v", told, err)
	}
	if want := "Posted 11 hours ago."; updates.recorded[0].Body != want {
		t.Errorf("body = %q, want %q", updates.recorded[0].Body, want)
	}
}

func TestAJobWithoutAPostedDateSaysWhenItWasFound(t *testing.T) {
	match := store.FreshMatch{JobID: uuid.New(), JobTitle: "Engineer", FirstSeenAt: at(t, "2026-10-02 09:00")}
	teller, _, updates := startTeller(t, "2026-10-04 10:00", match)
	if _, err := teller.TellOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(updates.recorded) != 1 {
		t.Fatalf("recorded %d updates", len(updates.recorded))
	}
	if update := updates.recorded[0]; update.Title != "Strong match: Engineer" || update.Body != "Found 2 days ago." {
		t.Errorf("update = %q %q", update.Title, update.Body)
	}
}

func TestAFailedRecordLeavesTheMatchToTellAgain(t *testing.T) {
	match := store.FreshMatch{JobID: uuid.New(), JobTitle: "Engineer", FirstSeenAt: at(t, "2026-10-04 09:00")}
	teller, hub, updates := startTeller(t, "2026-10-04 10:00", match)
	updates.fail = true
	if _, err := teller.TellOnce(context.Background()); err == nil {
		t.Fatal("a failed record should stop the pass")
	}
	if _, told := hub.told[match.JobID]; told {
		t.Error("a match whose update failed was marked told")
	}
	updates.fail = false
	if told, err := teller.TellOnce(context.Background()); err != nil || told != 1 {
		t.Errorf("the retry told %d, error %v", told, err)
	}
}

func TestFormatAge(t *testing.T) {
	for age, want := range map[time.Duration]string{
		-time.Minute:     "within the hour",
		20 * time.Minute: "within the hour",
		time.Hour:        "1 hour ago",
		23 * time.Hour:   "23 hours ago",
		24 * time.Hour:   "1 day ago",
		71 * time.Hour:   "2 days ago",
	} {
		if got := formatAge(age); got != want {
			t.Errorf("formatAge(%v) = %q, want %q", age, got, want)
		}
	}
}
