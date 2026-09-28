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
