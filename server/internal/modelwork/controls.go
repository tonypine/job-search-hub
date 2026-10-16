// Package modelwork is the owner's handle on the hub's model work: what is
// running and waiting, each kind's pause, and runs the owner starts.
package modelwork

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/modelruntime"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// ErrNotSetUp means the hub has no model routes, so nothing can run.
var ErrNotSetUp = errors.New("no model is routed for this work")

type runtimeStatus interface {
	Status() modelruntime.Status
}

type jobFactsReader interface {
	ReadJobNow(ctx context.Context, jobID uuid.UUID) error
}

// Controls keep each kind's pause in Queue, which holds the paused kinds'
// model calls; the hub's other background work asks IsPaused before it
// starts a run.
type Controls struct {
	Hub     *store.Store
	Queue   *modelqueue.Queue
	Runtime runtimeStatus
	// Facts is nil when job facts have no route.
	Facts jobFactsReader
	// Drain, when set, lists the Claude runs running.
	Drain *drain.Drain

	// pausing keeps the saved pauses and the queue's in the same order.
	pausing sync.Mutex
}

// Status is the queue's state with the runtime's, and how many jobs wait
// for facts from the latest prompt.
type Status struct {
	modelqueue.Status
	// Paused says every kind the hub starts by itself is paused.
	Paused bool `json:"paused"`
	// PausedKinds are the kinds paused.
	PausedKinds       []string            `json:"paused_kinds"`
	Runtime           modelruntime.Status `json:"runtime"`
	JobsAwaitingFacts int                 `json:"jobs_awaiting_facts"`
}

func (controls *Controls) GetStatus(ctx context.Context) (Status, error) {
	status := Status{Status: controls.Queue.Status(), Paused: true, PausedKinds: []string{}, Runtime: controls.Runtime.Status()}
	for _, kind := range store.ListAutomaticRunKindIDs() {
		if controls.Queue.IsPaused(kind) {
			status.PausedKinds = append(status.PausedKinds, kind)
		} else {
			status.Paused = false
		}
	}
	prompt, err := controls.Hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	if errors.Is(err, store.ErrAgentPromptNotFound) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	status.JobsAwaitingFacts, err = controls.Hub.CountJobsAwaitingFacts(ctx, prompt.ID)
	return status, err
}

// SetPaused pauses or resumes kinds: it saves the pauses, so they survive
// restarts, then applies them. A paused kind's running run finishes, and
// its waiting work stays queued. Kinds only the owner starts refuse a
// pause (store.ErrRunKindNotPausable).
func (controls *Controls) SetPaused(ctx context.Context, actor store.Actor, kinds []string, paused bool) error {
	controls.pausing.Lock()
	defer controls.pausing.Unlock()
	if err := controls.Hub.SetRunKindsPaused(ctx, actor, kinds, paused); err != nil {
		return err
	}
	for _, kind := range kinds {
		controls.Queue.SetPaused(kind, paused)
	}
	return nil
}

// SetAllPaused pauses or resumes every kind the hub starts by itself.
func (controls *Controls) SetAllPaused(ctx context.Context, actor store.Actor, paused bool) error {
	return controls.SetPaused(ctx, actor, store.ListAutomaticRunKindIDs(), paused)
}

// IsPaused says whether kind's background runs wait for a resume. Runs the
// owner starts don't ask.
func (controls *Controls) IsPaused(kind string) bool {
	return controls.Queue.IsPaused(kind)
}

// RunKindState is a kind of run with what it is doing now.
type RunKindState struct {
	store.RunKind
	Paused bool `json:"paused"`
	// Running counts its runs running now.
	Running int `json:"running"`
	// Waiting counts its model calls waiting for their turn.
	Waiting int `json:"waiting"`
}

// ListRunKinds is every kind of run, in the catalog's order, with its pause,
// its runs running now and its calls waiting.
func (controls *Controls) ListRunKinds(ctx context.Context) ([]RunKindState, error) {
	agentRuns, err := controls.Hub.ListRunningAgentRuns(ctx)
	if err != nil {
		return nil, err
	}
	queue := controls.Queue.Status()
	running := map[string]int{}
	if queue.Running != nil {
		running[queue.Running.Kind]++
	}
	// The drain lists the queue's running call too, counted above.
	for _, work := range controls.Drain.Running() {
		if work.Type != drain.TypeModelCall {
			running[work.Kind]++
		}
	}
	for _, run := range agentRuns {
		running[run.Kind]++
	}
	kinds := make([]RunKindState, 0, len(store.RunKinds))
	for _, kind := range store.RunKinds {
		kinds = append(kinds, RunKindState{
			RunKind: kind, Paused: kind.Automatic && controls.Queue.IsPaused(kind.ID),
			Running: running[kind.ID], Waiting: queue.WaitingByKind[kind.ID],
		})
	}
	return kinds, nil
}

// DispatchJobFacts starts reading the job's facts ahead of background work
// and returns once the run is queued; the job's facts change when it ends.
func (controls *Controls) DispatchJobFacts(ctx context.Context, jobID uuid.UUID) error {
	if controls.Facts == nil {
		return ErrNotSetUp
	}
	if _, err := controls.Hub.GetJobForFacts(ctx, jobID); err != nil {
		return err
	}
	go func() {
		if err := controls.Facts.ReadJobNow(context.WithoutCancel(ctx), jobID); err != nil {
			slog.Warn("dispatched job facts failed", "job", jobID, "error", err)
		}
	}()
	return nil
}
