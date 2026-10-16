package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

// finishAgentRunAfter finishes an agent run a while after it started, with
// the cost Claude reported.
func finishAgentRunAfter(t *testing.T, pool *pgxpool.Pool, hub *store.Store, id uuid.UUID, took time.Duration, cost string) {
	t.Helper()
	ctx := context.Background()
	if _, err := hub.FinishAgentRun(ctx, id, store.AgentRunOutcome{Status: store.AgentRunSucceeded, CostUSDEstimate: cost}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET finished_at = started_at + $2::double precision * interval '1 second' WHERE id = $1`, id,
		took.Seconds()); err != nil {
		t.Fatal(err)
	}
}

func TestRunMetricsSplitAWeekByDayAndKind(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// Clocks go forward on Sunday 8 March, so the week is 167 hours long.
	from, to := time.Date(2026, 3, 5, 0, 0, 0, 0, newYork), time.Date(2026, 3, 12, 0, 0, 0, 0, newYork)
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 3, day, hour, minute, 0, 0, newYork) }

	recordTaskRunAt(t, hub, store.AgentPromptKindJobBrief, nil, store.RunOutcomeSucceeded, from)
	recordTaskRunAt(t, hub, store.AgentPromptKindJobFacts, nil, store.RunOutcomeFailed, at(5, 10, 0))
	if _, err := hub.RecordTaskRun(ctx, store.NewTaskRun{
		Kind: store.RunKindJobBriefFull, BaseURL: "claude-cli", Provider: "Claude", Model: "sonnet", StartedAt: at(6, 9, 0),
		Duration: 4 * time.Second, Outcome: store.RunOutcomeSucceeded, PromptTokens: 1000, CompletionTokens: 300,
	}); err != nil {
		t.Fatal(err)
	}
	recordTaskRunAt(t, hub, store.AgentPromptKindHiringThread, nil, store.RunOutcomeInvalid, at(8, 12, 0))
	// 00:30 on the 9th is 04:30 UTC: a day of 24 hours from the 5th's
	// midnight would put it on the 8th.
	research := startAgentRunAt(t, pool, hub, store.AgentRunKindCompanyTriage, "acme.com", nil, at(9, 0, 30))
	finishAgentRunAfter(t, pool, hub, research.ID, time.Minute, "0.42")
	startAgentRunAt(t, pool, hub, store.AgentRunKindProfileSeed, "owner profile", nil, at(11, 23, 59))
	// The window ends before its to, and the one before starts 167 hours
	// before its from.
	recordTaskRunAt(t, hub, store.AgentPromptKindJobBrief, nil, store.RunOutcomeSucceeded, to)
	recordTaskRunAt(t, hub, store.AgentPromptKindJobBrief, nil, store.RunOutcomeSucceeded, at(1, 9, 0))
	recordTaskRunAt(t, hub, store.AgentPromptKindJobBrief, nil, store.RunOutcomeFailed, at(2, 9, 0))
	earlier := startAgentRunAt(t, pool, hub, store.AgentRunKindJobFinder, "acme.com", nil, at(3, 9, 0))
	finishAgentRunAfter(t, pool, hub, earlier.ID, time.Minute, "0.31")
	recordTaskRunAt(t, hub, store.AgentPromptKindJobBrief, nil, store.RunOutcomeSucceeded, time.Date(2026, 2, 26, 0, 30, 0, 0, newYork))

	metrics, err := hub.GetRunMetrics(ctx, store.RunMetricsFilter{From: from, To: to, Bucket: store.RunBucketDay, Location: newYork})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Kind != "" || metrics.Bucket != store.RunBucketDay || metrics.TimeZone != "America/New_York" {
		t.Errorf("metrics = %+v", metrics)
	}
	totals := metrics.Totals
	if !totals.From.Equal(from) || !totals.To.Equal(to) || totals.Runs != 6 || totals.Failed != 1 || totals.Invalid != 1 ||
		totals.InputTokens != 3700 || totals.OutputTokens != 660 || totals.CostUSD != "0.42" || totals.ClaudePlanRuns != 1 {
		t.Errorf("totals = %+v", totals)
	}
	// Five runs finished: three of 2 s, one of 4 s and one of a minute.
	if totals.AverageDurationMS == nil || *totals.AverageDurationMS != 14000 {
		t.Errorf("average duration = %v; want 14000 ms", totals.AverageDurationMS)
	}
	listed, err := hub.ListRuns(ctx, store.RunFilter{Since: &from, Until: &to, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Runs) != totals.Runs {
		t.Errorf("the window lists %d runs; its totals count %d", len(listed.Runs), totals.Runs)
	}
	previous := metrics.Previous
	if !previous.From.Equal(from.Add(-167*time.Hour)) || !previous.To.Equal(from) || previous.Runs != 3 || previous.Failed != 1 ||
		previous.CostUSD != "0.31" || previous.ClaudePlanRuns != 0 {
		t.Errorf("previous = %+v", previous)
	}

	type count struct {
		kind   string
		runs   int
		failed int
	}
	want := [][]count{
		{{store.AgentPromptKindJobBrief, 1, 0}, {store.AgentPromptKindJobFacts, 1, 1}},
		{{store.RunKindJobBriefFull, 1, 0}},
		{},
		{{store.AgentPromptKindHiringThread, 1, 0}},
		{{store.AgentRunKindCompanyTriage, 1, 0}},
		{},
		{{store.AgentRunKindProfileSeed, 1, 0}},
	}
	if len(metrics.Buckets) != len(want) {
		t.Fatalf("%d buckets; want %d: %+v", len(metrics.Buckets), len(want), metrics.Buckets)
	}
	runs, inputTokens := 0, int64(0)
	for index, bucket := range metrics.Buckets {
		start := time.Date(2026, 3, 5+index, 0, 0, 0, 0, newYork)
		if !bucket.Start.Equal(start) || !bucket.End.Equal(start.AddDate(0, 0, 1)) {
			t.Errorf("bucket %d runs from %v to %v; want the day from %v", index, bucket.Start, bucket.End, start)
		}
		if len(bucket.Kinds) != len(want[index]) {
			t.Errorf("bucket %d's kinds = %+v; want %+v", index, bucket.Kinds, want[index])
			continue
		}
		for position, kind := range bucket.Kinds {
			if expected := want[index][position]; kind.Kind != expected.kind || kind.Runs != expected.runs || kind.Failed != expected.failed {
				t.Errorf("bucket %d's kind %d = %+v; want %+v", index, position, kind, expected)
			}
		}
		runs += bucket.Runs
		inputTokens += bucket.InputTokens
	}
	if hours := metrics.Buckets[3].End.Sub(metrics.Buckets[3].Start); hours != 23*time.Hour {
		t.Errorf("8 March lasts %v; want 23h", hours)
	}
	if runs != totals.Runs || inputTokens != totals.InputTokens {
		t.Errorf("the buckets count %d runs and %d input tokens; the totals %d and %d", runs, inputTokens, totals.Runs, totals.InputTokens)
	}

	briefs, err := hub.GetRunMetrics(ctx, store.RunMetricsFilter{
		Kind: store.AgentPromptKindJobBrief, From: from, To: to, Bucket: store.RunBucketDay, Location: newYork,
	})
	if err != nil {
		t.Fatal(err)
	}
	if briefs.Totals.Runs != 1 || briefs.Previous.Runs != 2 || briefs.Previous.Failed != 1 || briefs.Totals.CostUSD != "0" ||
		briefs.Totals.AverageDurationMS == nil || *briefs.Totals.AverageDurationMS != 2000 {
		t.Errorf("job briefs: totals %+v, previous %+v", briefs.Totals, briefs.Previous)
	}
	if empty := briefs.Buckets[2]; empty.Runs != 0 || empty.Kinds == nil || len(empty.Kinds) != 0 {
		t.Errorf("an empty bucket = %+v", empty)
	}
	if found, err := hub.GetRunMetrics(ctx, store.RunMetricsFilter{
		Kind: store.AgentPromptKindMarketGaps, From: from, To: to, Bucket: store.RunBucketDay, Location: newYork,
	}); err != nil || found.Totals.Runs != 0 || found.Totals.AverageDurationMS != nil || len(found.Buckets) != 7 {
		t.Errorf("a kind that never ran: %+v, %v", found, err)
	}
}

func TestRunMetricsCount90DaysOf100kRunsQuickly(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Fatal(err)
	}
	to := time.Date(2026, 10, 16, 0, 0, 0, 0, lisbon)
	from := to.AddDate(0, 0, -90)
	// 98,000 model calls, one every 150 seconds over 170 days, and 2,000
	// agent runs, one every two hours.
	if _, err := pool.Exec(ctx, `
		INSERT INTO task_runs (kind, base_url, provider, model, started_at, duration_ms, outcome, prompt_tokens, completion_tokens)
		SELECT (ARRAY['job_facts', 'job_brief', 'mail_triage', 'job_cv'])[1 + n % 4], 'http://localhost:1234/v1', 'LM Studio', 'qwen',
			$1::timestamptz - n * interval '150 seconds', 1000 + n % 5000,
			CASE WHEN n % 20 = 0 THEN 'failed' WHEN n % 50 = 1 THEN 'invalid' ELSE 'succeeded' END, 800, 100
		FROM generate_series(1, 98000) n`, to); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_runs (kind, input, token_hash, token_expires_at, status, cost_usd_estimate, started_at, finished_at)
		SELECT 'company_triage', 'acme.com', decode(md5(n::text), 'hex'), now(), 'succeeded', 0.25,
			$1::timestamptz - n * interval '2 hours', $1::timestamptz - n * interval '2 hours' + interval '1 minute'
		FROM generate_series(1, 2000) n`, to); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE task_runs, agent_runs`); err != nil {
		t.Fatal(err)
	}
	var want int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM task_runs WHERE started_at >= $1 AND started_at < $2)
		+ (SELECT count(*) FROM agent_runs WHERE started_at >= $1 AND started_at < $2)`, from, to).Scan(&want); err != nil {
		t.Fatal(err)
	}

	filter := store.RunMetricsFilter{From: from, To: to, Bucket: store.RunBucketDay, Location: lisbon}
	if _, err := hub.GetRunMetrics(ctx, filter); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	metrics, err := hub.GetRunMetrics(ctx, filter)
	took := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if took >= 200*time.Millisecond {
		t.Errorf("90 days by day of 100k runs took %v; want under 200ms", took)
	}
	if len(metrics.Buckets) != 90 || metrics.Totals.Runs != want || metrics.Previous.Runs == 0 {
		t.Errorf("%d buckets, %d runs (want 90 and %d), %d before", len(metrics.Buckets), metrics.Totals.Runs, want, metrics.Previous.Runs)
	}
	t.Logf("90 days by day of 100k runs: %v", took)
}

// A model call a provider charged for adds its cost; one on the Claude plan
// adds nothing, whatever Claude reckons it would cost on its API.
func TestRunMetricsCountWhatAProviderChargedButNotTheClaudePlan(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	from := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	for _, run := range []store.NewTaskRun{
		{Kind: store.AgentPromptKindRecruiterScreen, BaseURL: "https://openrouter.ai/api/v1", Provider: "OpenRouter", Model: "anthropic/claude-sonnet-4.5",
			CostUSD: "0.0081", StartedAt: from.Add(time.Hour), Outcome: store.RunOutcomeSucceeded},
		{Kind: store.RunKindJobBriefFull, BaseURL: "claude-cli", Provider: "Claude", Model: "sonnet", CostUSD: "0.05", StartedAt: from.Add(2 * time.Hour),
			Outcome: store.RunOutcomeSucceeded},
	} {
		if _, err := hub.RecordTaskRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	metrics, err := hub.GetRunMetrics(ctx, store.RunMetricsFilter{From: from, To: from.Add(24 * time.Hour), Bucket: store.RunBucketDay})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Totals.Runs != 2 || metrics.Totals.CostUSD != "0.0081" || metrics.Totals.ClaudePlanRuns != 1 {
		t.Errorf("totals = %+v", metrics.Totals)
	}
}
