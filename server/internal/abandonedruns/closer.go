// Package abandonedruns closes the agent runs whose process died without
// reporting an end, so they stop showing as running for good.
package abandonedruns

import (
	"context"
	"log/slog"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type agentRuns interface {
	CloseAbandonedAgentRuns(ctx context.Context) ([]store.AgentRun, error)
}

type Closer struct {
	hub agentRuns
}

func NewCloser(hub agentRuns) *Closer {
	return &Closer{hub: hub}
}

// Run closes once at start, catching runs a restart cut short, then every
// interval, until ctx ends.
func (closer *Closer) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		closer.CloseOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// CloseOnce fails the runs still running after their token expired, and
// returns how many it closed.
func (closer *Closer) CloseOnce(ctx context.Context) int {
	closed, err := closer.hub.CloseAbandonedAgentRuns(ctx)
	if err != nil {
		slog.Error("close abandoned agent runs", "error", err)
		return 0
	}
	for _, run := range closed {
		slog.Info("closed an abandoned agent run", "id", run.ID, "kind", run.Kind, "input", run.Input, "token_expired_at", run.TokenExpiresAt)
	}
	return len(closed)
}
