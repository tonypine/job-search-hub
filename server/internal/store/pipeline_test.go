package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/migrations"
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

func TestAFollowUpFallsDueByPhaseAndRestartsWhenRecorded(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/jobs/1"})
	application, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})
	if err != nil {
		t.Fatal(err)
	}
	phases, _ := hub.ListPipelinePhases(ctx)
	byName := map[string]store.PipelinePhase{}
	for _, phase := range phases {
		byName[phase.Name] = phase
	}
	if byName["Saved"].FollowUpDays != nil || byName["Applied"].FollowUpDays == nil || *byName["Applied"].FollowUpDays != 7 {
		t.Fatalf("seeded intervals = Saved %v, Applied %v", byName["Saved"].FollowUpDays, byName["Applied"].FollowUpDays)
	}

	cards, _ := hub.ListPipelineCards(ctx)
	if cards[0].FollowUpDueAt != nil {
		t.Fatalf("a Saved card is due at %v; Saved asks for no follow-up", cards[0].FollowUpDueAt)
	}

	// Applied ten days ago: three days overdue.
	if _, err := hub.MoveApplication(ctx, owner, application.ID, byName["Applied"].ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE applications SET phase_entered_at = now() - interval '10 days' WHERE id = $1`, application.ID); err != nil {
		t.Fatal(err)
	}
	cards, _ = hub.ListPipelineCards(ctx)
	if due := cards[0].FollowUpDueAt; due == nil || time.Since(*due) < 71*time.Hour || time.Since(*due) > 73*time.Hour {
		t.Fatalf("due at %v; want three days ago", due)
	}

	if _, err := hub.RecordFollowUp(ctx, owner, application.ID, "Pinged the recruiter."); err != nil {
		t.Fatal(err)
	}
	cards, _ = hub.ListPipelineCards(ctx)
	if due := cards[0].FollowUpDueAt; due == nil || time.Until(*due) < 167*time.Hour {
		t.Fatalf("after a follow-up, due at %v; want in seven days", due)
	}
	var notes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM changes WHERE operation = 'follow_up' AND after->>'note' = 'Pinged the recruiter.'`).Scan(&notes); err != nil || notes != 1 {
		t.Fatalf("follow-up changes = %d, %v", notes, err)
	}

	days := 3
	if phase, err := hub.SetPipelinePhaseFollowUpDays(ctx, owner, byName["Applied"].ID, &days); err != nil || *phase.FollowUpDays != 3 {
		t.Fatalf("set 3 days = %+v, %v", phase, err)
	}
	if phase, err := hub.SetPipelinePhaseFollowUpDays(ctx, owner, byName["Applied"].ID, nil); err != nil || phase.FollowUpDays != nil {
		t.Fatalf("clear = %+v, %v", phase, err)
	}
	zero := 0
	if _, err := hub.SetPipelinePhaseFollowUpDays(ctx, owner, byName["Applied"].ID, &zero); err == nil {
		t.Fatal("zero days was accepted")
	}
}

func TestACompanysApplicationsComeWithTheirPhaseAndLeaveOtherCompaniesOut(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	acme, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	zeta, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Zeta", Domain: "zeta.com"})
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &acme.ID, Title: "Engineer", URL: "https://acme.com/jobs/1"})
	if _, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID}); err != nil {
		t.Fatal(err)
	}
	outreach, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &acme.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &zeta.ID}); err != nil {
		t.Fatal(err)
	}
	phases, _ := hub.ListPipelinePhases(ctx)
	closed := phases[len(phases)-1]
	if _, err := hub.MoveApplication(ctx, owner, outreach.ID, closed.ID, "No answer."); err != nil {
		t.Fatal(err)
	}

	applications, err := hub.ListCompanyApplications(ctx, acme.ID)
	if err != nil || len(applications) != 2 {
		t.Fatalf("acme's applications = %+v, %v; want 2", applications, err)
	}
	byTitle := map[string]store.CompanyApplication{}
	for _, application := range applications {
		if application.Application.CompanyID == nil || *application.Application.CompanyID != acme.ID {
			t.Fatalf("listed another company's card: %+v", application)
		}
		title := "outreach"
		if application.JobTitle != nil {
			title = *application.JobTitle
		}
		byTitle[title] = application
	}
	if saved := byTitle["Engineer"]; saved.PhaseName != "Saved" || saved.PhaseIsClosed {
		t.Fatalf("the job's card = %+v; want it in Saved", saved)
	}
	if ended := byTitle["outreach"]; ended.PhaseName != closed.Name || !ended.PhaseIsClosed || ended.Application.ClosedReason != "No answer." {
		t.Fatalf("the outreach card = %+v; want it closed", ended)
	}

	none, err := hub.ListCompanyApplications(ctx, uuid.New())
	if err != nil || len(none) != 0 {
		t.Fatalf("an unknown company's applications = %+v, %v; want none", none, err)
	}
}

func sameTime(got *time.Time, want time.Time) bool {
	return got != nil && got.Equal(want)
}

func TestAnApplicationIsDatedGoneOutTheFirstTimeItReachesApplied(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	phases, _ := hub.ListPipelinePhases(ctx)
	saved, applied, inContact, closed := phases[0], phases[1], phases[2], phases[5]
	sentAt := time.Now().Add(-10 * 24 * time.Hour).UTC().Truncate(time.Second)

	sent, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID})
	if sent.AppliedAt != nil {
		t.Fatalf("a new card went out at %v; it's only saved", sent.AppliedAt)
	}
	moved, err := hub.MoveApplicationAsOf(ctx, owner, sent.ID, applied.ID, "", sentAt, "")
	if err != nil || !sameTime(moved.AppliedAt, sentAt) {
		t.Fatalf("move to Applied = %+v, %v; want it gone out at %v", moved.AppliedAt, err, sentAt)
	}
	for _, phase := range []store.PipelinePhase{inContact, saved, applied, closed} {
		if moved, err = hub.MoveApplication(ctx, owner, sent.ID, phase.ID, "Rejected."); err != nil || !sameTime(moved.AppliedAt, sentAt) {
			t.Fatalf("move to %s = %+v, %v; want it still gone out at %v", phase.Name, moved.AppliedAt, err, sentAt)
		}
	}

	dropped, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID})
	if dropped, err = hub.MoveApplication(ctx, owner, dropped.ID, closed.ID, "Not for me."); err != nil || dropped.AppliedAt != nil {
		t.Fatalf("closed from Saved = %+v, %v; want it never gone out", dropped.AppliedAt, err)
	}

	cards, _ := hub.ListPipelineCards(ctx)
	for _, card := range cards {
		if card.Application.ID == sent.ID && !sameTime(card.Application.AppliedAt, sentAt) {
			t.Fatalf("the board lists the closed card gone out at %v; want %v", card.Application.AppliedAt, sentAt)
		}
	}
}

func TestAMailConfirmationDatesTheApplicationGoneOutByTheMail(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	phases, _ := hub.ListPipelinePhases(ctx)
	applied := phases[1]
	mailedAt := time.Now().Add(-3 * 24 * time.Hour).UTC().Truncate(time.Second)

	// A confirmation for a saved card moves it to Applied at the mail's date.
	saved, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID})
	confirmed, err := hub.MoveApplicationAsOf(ctx, owner, saved.ID, applied.ID, "", mailedAt, "https://mail.google.com/mail/#all/1")
	if err != nil || !sameTime(confirmed.AppliedAt, mailedAt) {
		t.Fatalf("confirmed = %+v, %v; want it gone out at %v", confirmed.AppliedAt, err, mailedAt)
	}

	// One the owner moved to Applied after sending it goes back to the mail's date.
	late, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID})
	if late, err = hub.MoveApplication(ctx, owner, late.ID, applied.ID, ""); err != nil {
		t.Fatal(err)
	}
	corrected, changed, err := hub.CorrectApplicationPhaseEnteredAt(ctx, owner, late.ID, mailedAt, "https://mail.google.com/mail/#all/2")
	if err != nil || !changed || !sameTime(corrected.AppliedAt, mailedAt) {
		t.Fatalf("corrected = %+v, %v, %v; want it gone out at %v", corrected.AppliedAt, changed, err, mailedAt)
	}
	later := mailedAt.Add(24 * time.Hour)
	if again, _, err := hub.CorrectApplicationPhaseEnteredAt(ctx, owner, late.ID, later, ""); err != nil || !sameTime(again.AppliedAt, mailedAt) {
		t.Fatalf("a later mail = %+v, %v; want it still gone out at %v", again.AppliedAt, err, mailedAt)
	}

	// A card still saved is dated in its phase but hasn't gone out.
	stillSaved, _, _ := hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID})
	if stillSaved, _, err = hub.CorrectApplicationPhaseEnteredAt(ctx, owner, stillSaved.ID, mailedAt, ""); err != nil || stillSaved.AppliedAt != nil {
		t.Fatalf("a saved card = %+v, %v; want it never gone out", stillSaved.AppliedAt, err)
	}
}

// The migration that adds applied_at, which the backfill test rolls back and
// applies again.
const appliedAtMigration = 83

func TestExistingApplicationsAreDatedGoneOutFromTheChangeLog(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	database := stdlib.OpenDBFromPool(pool)
	defer database.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, appliedAtMigration-1); err != nil {
		t.Fatal(err)
	}

	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	phases, _ := hub.ListPipelinePhases(ctx)
	saved, applied, interviewing, closed := phases[0], phases[1], phases[3], phases[5]
	now := time.Now().UTC().Truncate(time.Second)
	day := func(daysAgo int) time.Time { return now.AddDate(0, 0, -daysAgo) }
	addCard := func(phase store.PipelinePhase, enteredAt time.Time) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO applications (company_id, phase_id, phase_entered_at) VALUES ($1, $2, $3) RETURNING id`,
			company.ID, phase.ID, enteredAt).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	logChange := func(id uuid.UUID, operation string, after map[string]any, at time.Time) {
		recorded, _ := json.Marshal(after)
		if _, err := pool.Exec(ctx, `INSERT INTO changes (actor_kind, entity_type, entity_id, operation, after, created_at)
			VALUES ('owner', 'application', $1, $2, $3, $4)`, id, operation, recorded, at); err != nil {
			t.Fatal(err)
		}
	}
	move := func(id uuid.UUID, phase store.PipelinePhase, at time.Time) {
		logChange(id, "move", map[string]any{"phase_id": phase.ID, "closed_reason": ""}, at)
	}

	rejected := addCard(closed, day(2))
	move(rejected, applied, day(20))
	move(rejected, interviewing, day(10))
	move(rejected, closed, day(2))
	dropped := addCard(closed, day(5))
	move(dropped, closed, day(5))
	backToSaved := addCard(saved, day(4))
	move(backToSaved, applied, day(8))
	move(backToSaved, saved, day(4))
	// Moved to Applied by hand, then dated back by the confirming mail.
	confirmed := addCard(applied, day(12))
	move(confirmed, applied, day(9))
	unlogged := addCard(interviewing, day(6))
	imported := addCard(closed, day(200))
	logChange(imported, "create", map[string]any{"phase_id": closed.ID, "phase_entered_at": day(200)}, day(1))

	if _, err := provider.UpTo(ctx, appliedAtMigration); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[uuid.UUID]time.Time{rejected: day(20), backToSaved: day(8), confirmed: day(12), unlogged: day(6), imported: day(200),
		dropped: {}} {
		var got *time.Time
		if err := pool.QueryRow(ctx, `SELECT applied_at FROM applications WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (want.IsZero() && got != nil) || (!want.IsZero() && !sameTime(got, want)) {
			t.Errorf("card %s gone out at %v; want %v", id, got, want)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
}
