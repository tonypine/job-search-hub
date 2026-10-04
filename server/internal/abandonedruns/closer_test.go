package abandonedruns

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type fakeRuns struct {
	closed []store.AgentRun
	err    error
	calls  chan struct{}
}

func (fake *fakeRuns) CloseAbandonedAgentRuns(context.Context) ([]store.AgentRun, error) {
	if fake.calls != nil {
		fake.calls <- struct{}{}
	}
	return fake.closed, fake.err
}

func TestCloseOnceCountsTheClosedRuns(t *testing.T) {
	hub := &fakeRuns{closed: []store.AgentRun{{ID: uuid.New()}, {ID: uuid.New()}}}
	if got := NewCloser(hub).CloseOnce(context.Background()); got != 2 {
		t.Fatalf("closed %d, want 2", got)
	}
}

func TestCloseOnceSurvivesAFailedPass(t *testing.T) {
	hub := &fakeRuns{err: errors.New("database down")}
	if got := NewCloser(hub).CloseOnce(context.Background()); got != 0 {
		t.Fatalf("closed %d, want 0", got)
	}
}

func TestRunClosesAtStartAndOnEachTickUntilCancelled(t *testing.T) {
	hub := &fakeRuns{calls: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		NewCloser(hub).Run(ctx, 10*time.Millisecond)
		close(stopped)
	}()

	for pass := range 3 {
		select {
		case <-hub.calls:
		case <-time.After(time.Second):
			t.Fatalf("pass %d never came", pass)
		}
	}
	cancel()
	// A tick may already be due; let its pass report before the run stops.
	for {
		select {
		case <-hub.calls:
		case <-stopped:
			return
		case <-time.After(time.Second):
			t.Fatal("Run didn't stop after its context ended")
		}
	}
}
