package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/migrations"
)

// startAgentRunAt starts an agent run and dates its start.
func startAgentRunAt(t *testing.T, pool *pgxpool.Pool, hub *store.Store, kind, input string, subjectID *uuid.UUID, at time.Time) store.AgentRun {
	t.Helper()
	ctx := context.Background()
	run, err := hub.StartAgentRun(ctx, kind, input, subjectID, "", 1, []byte(uuid.NewString()), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2 WHERE id = $1`, run.ID, at); err != nil {
		t.Fatal(err)
	}
	return run
}

func recordTaskRunAt(t *testing.T, hub *store.Store, kind string, subjectID *uuid.UUID, outcome string, at time.Time) store.TaskRun {
	t.Helper()
	run, err := hub.RecordTaskRun(context.Background(), store.NewTaskRun{
		Kind: kind, SubjectID: subjectID, Provider: "Local model", Model: "qwen", StartedAt: at, Duration: 2 * time.Second, Outcome: outcome,
		PromptTokens: 900, CompletionTokens: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestRunsPageWithACursorWithoutRepeatsOrGaps(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	var want []uuid.UUID
	// Runs of both records interleave, and some start at the same moment,
	// so the page boundaries fall between runs that tie.
	for index := range 7 {
		at := base.Add(time.Duration(index/2) * time.Minute)
		want = append(want, recordTaskRunAt(t, hub, store.AgentPromptKindJobFacts, nil, store.RunOutcomeSucceeded, at).ID)
	}
	for index := range 5 {
		at := base.Add(time.Duration(index) * time.Minute)
		want = append(want, startAgentRunAt(t, pool, hub, store.AgentRunKindCompanyTriage, "acme.com", nil, at).ID)
	}

	seen := map[uuid.UUID]bool{}
	var listed []store.Run
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("the pages never end")
		}
		page, err := hub.ListRuns(ctx, store.RunFilter{Limit: 5, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range page.Runs {
			if seen[run.ID] {
				t.Errorf("run %s listed twice", run.ID)
			}
			seen[run.ID] = true
		}
		listed = append(listed, page.Runs...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(listed) != len(want) {
		t.Fatalf("listed %d runs; want %d", len(listed), len(want))
	}
	for _, id := range want {
		if !seen[id] {
			t.Errorf("run %s skipped", id)
		}
	}
	for index := 1; index < len(listed); index++ {
		previous, current := listed[index-1], listed[index]
		if current.StartedAt.After(previous.StartedAt) {
			t.Errorf("run %d starts after the one before it: %v > %v", index, current.StartedAt, previous.StartedAt)
		}
	}
	if _, err := hub.ListRuns(ctx, store.RunFilter{Cursor: "not a cursor"}); !errors.Is(err, store.ErrInvalidRunCursor) {
		t.Errorf("a made-up cursor: %v", err)
	}
}

func TestRunsFilterByKindSubjectOutcomeAndTime(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &company.ID, Title: "Frontend Engineer", URL: "https://acme.com/jobs/1"})
	if err != nil {
		t.Fatal(err)
	}
	message, _, err := hub.RecordMailMessage(ctx, store.NewMailMessage{
		GmailMessageID: "m1", ThreadID: "t1", Direction: "received", Subject: "Your application", SentAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	brief := recordTaskRunAt(t, hub, store.AgentPromptKindJobBrief, &job.ID, store.RunOutcomeSucceeded, base)
	sorting := recordTaskRunAt(t, hub, store.AgentPromptKindMailTriage, &message.ID, store.RunOutcomeFailed, base.Add(time.Hour))
	thread := recordTaskRunAt(t, hub, store.AgentPromptKindHiringThread, nil, store.RunOutcomeInvalid, base.Add(2*time.Hour))
	research := startAgentRunAt(t, pool, hub, store.AgentRunKindCompanyTriage, "acme.com", &company.ID, base.Add(3*time.Hour))
	if _, err := hub.FinishAgentRun(ctx, research.ID, store.AgentRunOutcome{Status: store.AgentRunSucceeded, CostUSDEstimate: "0.42"}); err != nil {
		t.Fatal(err)
	}
	seed := startAgentRunAt(t, pool, hub, store.AgentRunKindProfileSeed, "owner profile", nil, base.Add(4*time.Hour))

	list := func(filter store.RunFilter) []store.Run {
		t.Helper()
		page, err := hub.ListRuns(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		return page.Runs
	}
	ids := func(runs []store.Run) []uuid.UUID {
		listed := make([]uuid.UUID, 0, len(runs))
		for _, run := range runs {
			listed = append(listed, run.ID)
		}
		return listed
	}
	same := func(got, want []uuid.UUID) bool {
		if len(got) != len(want) {
			return false
		}
		for index := range got {
			if got[index] != want[index] {
				return false
			}
		}
		return true
	}

	briefs := list(store.RunFilter{Kind: store.AgentPromptKindJobBrief})
	if !same(ids(briefs), []uuid.UUID{brief.ID}) {
		t.Fatalf("job briefs = %+v", briefs)
	}
	if subject := briefs[0].Subject; subject == nil || subject.Type != store.RunSubjectJob || *subject.ID != job.ID ||
		subject.Title != "Frontend Engineer" || subject.CompanyID == nil || *subject.CompanyID != company.ID || subject.CompanyName != "Acme" {
		t.Errorf("a brief's subject = %+v", subject)
	}
	if run := briefs[0]; run.Record != store.RunRecordModelCall || run.Provider != "Local model" || run.Model != "qwen" ||
		run.DurationMS == nil || *run.DurationMS != 2000 || run.PromptTokens != 900 || run.StartedByOwner {
		t.Errorf("a brief = %+v", run)
	}

	forCompany := list(store.RunFilter{SubjectID: &company.ID})
	if !same(ids(forCompany), []uuid.UUID{research.ID}) {
		t.Fatalf("runs for the company = %+v", forCompany)
	}
	if run := forCompany[0]; run.Subject == nil || run.Subject.Type != store.RunSubjectCompany || run.Subject.Title != "Acme" ||
		run.Subject.CompanyID != nil || run.Record != store.RunRecordAgentRun || run.Provider != "Claude" || !run.StartedByOwner ||
		run.Outcome != store.RunOutcomeSucceeded || run.CostUSDEstimate == nil || *run.CostUSDEstimate != "0.42" || run.DurationMS == nil {
		t.Errorf("the research run = %+v", run)
	}
	if runs := list(store.RunFilter{SubjectType: store.RunSubjectJob, SubjectID: &company.ID}); len(runs) != 0 {
		t.Errorf("the company's id as a job's = %+v", runs)
	}

	if runs := list(store.RunFilter{Outcome: store.RunOutcomeFailed}); !same(ids(runs), []uuid.UUID{sorting.ID}) {
		t.Errorf("failed runs = %+v", runs)
	} else if runs[0].Subject == nil || runs[0].Subject.Title != "Your application" {
		t.Errorf("the mail run's subject = %+v", runs[0].Subject)
	}
	if runs := list(store.RunFilter{Outcome: store.RunOutcomeRunning}); !same(ids(runs), []uuid.UUID{seed.ID}) {
		t.Errorf("running runs = %+v", runs)
	} else if runs[0].Subject == nil || runs[0].Subject.Type != store.RunSubjectProfile || runs[0].Subject.ID != nil || runs[0].DurationMS != nil {
		t.Errorf("the seed run = %+v", runs[0])
	}

	since, until := base.Add(time.Hour), base.Add(3*time.Hour)
	if runs := list(store.RunFilter{Since: &since, Until: &until}); !same(ids(runs), []uuid.UUID{thread.ID, sorting.ID}) {
		t.Errorf("runs from 10:00 until 12:00 = %+v", runs)
	} else if runs[0].Subject != nil {
		t.Errorf("a thread run has a subject: %+v", runs[0].Subject)
	}
	if runs := list(store.RunFilter{}); !same(ids(runs), []uuid.UUID{seed.ID, research.ID, thread.ID, sorting.ID, brief.ID}) {
		t.Errorf("every run = %+v", runs)
	}
}

func TestACompanyRunIsLinkedToTheCompanyItAdded(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	run := startAgentRunAt(t, pool, hub, store.AgentRunKindCompanyTriage, "Newco", nil, time.Now())
	if run.SubjectType != nil || run.SubjectID != nil {
		t.Fatalf("a run for an unknown company has a subject: %+v", run)
	}
	company, _, err := hub.CreateCompany(ctx, store.Actor{Kind: store.ActorAgentRun, AgentRunID: run.ID}, store.NewCompany{Name: "Newco", Domain: "newco.example"})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := hub.FinishAgentRun(ctx, run.ID, store.AgentRunOutcome{Status: store.AgentRunSucceeded})
	if err != nil {
		t.Fatal(err)
	}
	if finished.SubjectType == nil || *finished.SubjectType != store.RunSubjectCompany || finished.SubjectID == nil || *finished.SubjectID != company.ID {
		t.Errorf("the finished run = %+v", finished)
	}
}

// The migration that links runs to their subjects, which the backfill test
// rolls back and applies again.
const runSubjectsMigration = 87

func TestRunsRecordedBeforeSubjectsAreLinkedWhereTheyCanBe(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	database := stdlib.OpenDBFromPool(pool)
	defer database.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, runSubjectsMigration-1); err != nil {
		t.Fatal(err)
	}

	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	other, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Globex", Domain: "globex.com"})
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &company.ID, Title: "Frontend Engineer", URL: "https://acme.com/jobs/1"})
	if _, err := pool.Exec(ctx, `INSERT INTO model_providers (kind, name, base_url) VALUES
		('openai_compatible', 'LM Studio', 'http://localhost:1234/v1'), ('hub_runtime', 'This Mac', '')`); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	addTaskRun := func(kind, baseURL string, subjectID *uuid.UUID) uuid.UUID {
		t.Helper()
		at = at.Add(time.Minute)
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO task_runs (kind, subject_id, base_url, model, started_at, duration_ms, outcome)
			VALUES ($1, $2, $3, 'm', $4, 1000, 'succeeded') RETURNING id`, kind, subjectID, baseURL, at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	addAgentRun := func(kind, input string) uuid.UUID {
		t.Helper()
		at = at.Add(time.Minute)
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO agent_runs (kind, input, token_hash, token_expires_at, started_at)
			VALUES ($1, $2, $3, now(), $4) RETURNING id`, kind, input, []byte(uuid.NewString()), at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	fullBrief := addTaskRun(store.AgentPromptKindJobBrief, "claude-cli", &job.ID)
	routedBrief := addTaskRun(store.AgentPromptKindJobBrief, "http://localhost:1234/v1/", &job.ID)
	onThisMac := addTaskRun(store.AgentPromptKindJobFacts, "http://127.0.0.1:8089/v1", &job.ID)
	thread := addTaskRun(store.AgentPromptKindHiringThread, "http://gone.example/v1", nil)
	byDomain := addAgentRun(store.AgentRunKindCompanyTriage, "acme.com")
	byURL := addAgentRun(store.AgentRunKindJobFinder, "https://www.globex.com/careers")
	byName := addAgentRun(store.AgentRunKindCompanyTriage, " globex ")
	byID := addAgentRun(store.AgentRunKindJobFinder, company.ID.String())
	byChange := addAgentRun(store.AgentRunKindCompanyTriage, "the company behind acme")
	if _, err := pool.Exec(ctx, `INSERT INTO changes (actor_kind, agent_run_id, entity_type, entity_id, operation)
		VALUES ('agent_run', $1, 'company', $2, 'update')`, byChange, other.ID); err != nil {
		t.Fatal(err)
	}
	unknown := addAgentRun(store.AgentRunKindCompanyTriage, "Initech")
	fix := addAgentRun(store.AgentRunKindJobFix, job.ID.String())
	brokenFix := addAgentRun(store.AgentRunKindJobFix, "not a job")
	seed := addAgentRun(store.AgentRunKindProfileSeed, "owner profile")

	// The migration links the runs; the rest bring the columns ListRuns reads.
	if _, err := provider.UpTo(ctx, runSubjectsMigration); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := hub.ListRuns(ctx, store.RunFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	runs := map[uuid.UUID]store.Run{}
	for _, run := range page.Runs {
		runs[run.ID] = run
	}
	if len(runs) != 13 {
		t.Fatalf("listed %d runs; want all 13", len(runs))
	}
	subjectOf := func(id uuid.UUID) (string, uuid.UUID) {
		subject := runs[id].Subject
		if subject == nil {
			return "", uuid.UUID{}
		}
		if subject.ID == nil {
			return subject.Type, uuid.UUID{}
		}
		return subject.Type, *subject.ID
	}
	for id, want := range map[uuid.UUID]struct {
		subjectType string
		subjectID   uuid.UUID
	}{
		fullBrief: {"job", job.ID}, routedBrief: {"job", job.ID}, onThisMac: {"job", job.ID}, thread: {},
		byDomain: {"company", company.ID}, byURL: {"company", other.ID}, byName: {"company", other.ID}, byID: {"company", company.ID},
		byChange: {"company", other.ID}, unknown: {}, fix: {"job", job.ID}, brokenFix: {}, seed: {"profile", uuid.UUID{}},
	} {
		if subjectType, subjectID := subjectOf(id); subjectType != want.subjectType || subjectID != want.subjectID {
			t.Errorf("run %s (%s, %q) is for %s %s; want %s %s", id, runs[id].Kind, runs[id].Provider, subjectType, subjectID,
				want.subjectType, want.subjectID)
		}
	}
	if runs[fullBrief].Kind != store.RunKindJobBriefFull || runs[routedBrief].Kind != store.AgentPromptKindJobBrief {
		t.Errorf("brief kinds = %s, %s", runs[fullBrief].Kind, runs[routedBrief].Kind)
	}
	for id, want := range map[uuid.UUID]string{fullBrief: "Claude", routedBrief: "LM Studio", onThisMac: "This Mac", thread: ""} {
		if runs[id].Provider != want {
			t.Errorf("run %s's provider = %q; want %q", runs[id].Kind, runs[id].Provider, want)
		}
	}
	if subject := runs[fix].Subject; subject == nil || subject.Title != "Frontend Engineer" || subject.CompanyName != "Acme" {
		t.Errorf("the fix's subject = %+v", subject)
	}
}

func TestARunKeepsItsCostItsFallbackAndAnAgentsModel(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	recorded, err := hub.RecordTaskRun(ctx, store.NewTaskRun{
		Kind: store.AgentPromptKindRecruiterScreen, Provider: "Local model", Model: "qwen", StartedAt: at, Duration: time.Second,
		Outcome: store.RunOutcomeSucceeded, CostUSD: "0.0001234", FallbackFor: "OpenRouter",
	})
	if err != nil || recorded.CostUSD == nil || *recorded.CostUSD != "0.0001234" || recorded.FallbackFor != "OpenRouter" {
		t.Fatalf("recorded = %+v, %v", recorded, err)
	}
	unpriced, err := hub.RecordTaskRun(ctx, store.NewTaskRun{Kind: store.AgentPromptKindJobFacts, StartedAt: at.Add(time.Minute), Outcome: store.RunOutcomeSucceeded})
	if err != nil || unpriced.CostUSD != nil || unpriced.FallbackFor != "" {
		t.Fatalf("a run with no cost = %+v, %v", unpriced, err)
	}
	run, err := hub.StartAgentRun(ctx, store.AgentRunKindProfileSeed, "owner profile", nil, "opus", 1, []byte(uuid.NewString()), time.Now().Add(time.Hour))
	if err != nil || run.Model != "opus" {
		t.Fatalf("agent run = %+v, %v", run, err)
	}

	page, err := hub.ListRuns(ctx, store.RunFilter{})
	if err != nil || len(page.Runs) != 3 {
		t.Fatalf("runs = %+v, %v", page, err)
	}
	byID := map[uuid.UUID]store.Run{}
	for _, listed := range page.Runs {
		byID[listed.ID] = listed
	}
	if listed := byID[recorded.ID]; listed.CostUSDEstimate == nil || *listed.CostUSDEstimate != "0.0001234" || listed.FallbackFor != "OpenRouter" {
		t.Errorf("the model call = %+v", listed)
	}
	if listed := byID[run.ID]; listed.Model != "opus" {
		t.Errorf("the agent run = %+v", listed)
	}
}
