package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func startRun(t *testing.T, hub *store.Store, tokenHash string, expiresAt time.Time) store.AgentRun {
	t.Helper()
	run, err := hub.StartAgentRun(context.Background(), store.AgentRunKindCompanyTriage, "stripe.com", 1, []byte(tokenHash), expiresAt)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return run
}

func TestFinishAgentRunRecordsTheOutcomeOnce(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	run := startRun(t, hub, "hash-1", time.Now().Add(time.Hour))
	if run.Status != store.AgentRunRunning || run.FinishedAt != nil {
		t.Fatalf("started run = %+v", run)
	}

	finished, err := hub.FinishAgentRun(ctx, run.ID, store.AgentRunOutcome{
		Status:          store.AgentRunSucceeded,
		ClaudeSessionID: "session-1",
		CostUSDEstimate: "0.4217",
		Result:          json.RawMessage(`{"people_added":2}`),
	})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if finished.Status != store.AgentRunSucceeded || finished.ClaudeSessionID != "session-1" || finished.FinishedAt == nil {
		t.Fatalf("finished run = %+v", finished)
	}
	if finished.CostUSDEstimate == nil || *finished.CostUSDEstimate != "0.4217" {
		t.Fatalf("cost = %v, want exactly 0.4217", finished.CostUSDEstimate)
	}
	var result map[string]int
	if err := json.Unmarshal(finished.Result, &result); err != nil || result["people_added"] != 2 {
		t.Fatalf("result = %s, err = %v", finished.Result, err)
	}

	if _, err := hub.FinishAgentRun(ctx, run.ID, store.AgentRunOutcome{Status: store.AgentRunFailed}); !errors.Is(err, store.ErrAgentRunFinished) {
		t.Fatalf("second finish err = %v, want ErrAgentRunFinished", err)
	}
	if _, err := hub.FinishAgentRun(ctx, uuid.New(), store.AgentRunOutcome{Status: store.AgentRunFailed}); !errors.Is(err, store.ErrAgentRunNotFound) {
		t.Fatalf("unknown run err = %v, want ErrAgentRunNotFound", err)
	}
	if _, err := hub.FinishAgentRun(ctx, run.ID, store.AgentRunOutcome{Status: store.AgentRunRunning}); err == nil {
		t.Fatal("expected an error for finishing as running")
	}
}

func TestOnlyRunningUnexpiredRunsAreFoundByTokenHash(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	running := startRun(t, hub, "running", time.Now().Add(time.Hour))
	startRun(t, hub, "expired", time.Now().Add(-time.Minute))
	finished := startRun(t, hub, "finished", time.Now().Add(time.Hour))
	if _, err := hub.FinishAgentRun(ctx, finished.ID, store.AgentRunOutcome{Status: store.AgentRunFailed}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	found, err := hub.GetRunningAgentRunByTokenHash(ctx, []byte("running"))
	if err != nil || found.ID != running.ID {
		t.Fatalf("running: found=%v err=%v", found.ID, err)
	}
	for _, hash := range []string{"expired", "finished", "unknown"} {
		if _, err := hub.GetRunningAgentRunByTokenHash(ctx, []byte(hash)); !errors.Is(err, store.ErrAgentRunNotFound) {
			t.Errorf("%s: err = %v, want ErrAgentRunNotFound", hash, err)
		}
	}
}

func TestOnlyRunningUnexpiredRunsAreListedAsRunning(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	first := startRun(t, hub, "first", time.Now().Add(time.Hour))
	second := startRun(t, hub, "second", time.Now().Add(time.Hour))
	startRun(t, hub, "expired", time.Now().Add(-time.Minute))
	finished := startRun(t, hub, "finished", time.Now().Add(time.Hour))
	if _, err := hub.FinishAgentRun(ctx, finished.ID, store.AgentRunOutcome{Status: store.AgentRunSucceeded}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	running, err := hub.ListRunningAgentRuns(ctx)
	if err != nil || len(running) != 2 || running[0].ID != first.ID || running[1].ID != second.ID {
		t.Fatalf("running = %+v, err = %v, want the two unexpired runs, oldest first", running, err)
	}
}

func TestCloseAbandonedAgentRunsFailsOnlyExpiredRunningRuns(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	abandoned := startRun(t, hub, "abandoned", time.Now().Add(-time.Minute))
	running := startRun(t, hub, "running", time.Now().Add(time.Hour))
	finished := startRun(t, hub, "finished", time.Now().Add(-time.Minute))
	if _, err := hub.FinishAgentRun(ctx, finished.ID, store.AgentRunOutcome{Status: store.AgentRunSucceeded}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	closed, err := hub.CloseAbandonedAgentRuns(ctx)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if len(closed) != 1 || closed[0].ID != abandoned.ID {
		t.Fatalf("closed = %+v, want only the abandoned run", closed)
	}
	if closed[0].Status != store.AgentRunFailed || closed[0].Error != store.AgentRunAbandoned || closed[0].FinishedAt == nil {
		t.Fatalf("closed run = %+v, want failed as abandoned", closed[0])
	}
	if got, err := hub.GetAgentRun(ctx, running.ID); err != nil || got.Status != store.AgentRunRunning || got.FinishedAt != nil {
		t.Fatalf("running run = %+v, err = %v, want it left running", got, err)
	}
	if got, err := hub.GetAgentRun(ctx, finished.ID); err != nil || got.Status != store.AgentRunSucceeded || got.Error != "" {
		t.Fatalf("finished run = %+v, err = %v, want it left as it ended", got, err)
	}

	again, err := hub.CloseAbandonedAgentRuns(ctx)
	if err != nil || len(again) != 0 {
		t.Fatalf("second pass closed %+v, err = %v, want nothing", again, err)
	}
}
