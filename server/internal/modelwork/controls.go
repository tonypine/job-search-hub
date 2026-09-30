// Package modelwork is the owner's handle on the hub's model work: what is
// running and waiting, the pause, and runs the owner starts.
package modelwork

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

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

type Controls struct {
	Hub     *store.Store
	Queue   *modelqueue.Queue
	Runtime runtimeStatus
	// Facts is nil when job facts have no route.
	Facts jobFactsReader
}

// Status is the queue's state with the runtime's, and how many jobs wait
// for facts from the latest prompt.
type Status struct {
	modelqueue.Status
	Runtime           modelruntime.Status `json:"runtime"`
	JobsAwaitingFacts int                 `json:"jobs_awaiting_facts"`
}

func (controls *Controls) GetStatus(ctx context.Context) (Status, error) {
	status := Status{Status: controls.Queue.Status(), Runtime: controls.Runtime.Status()}
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

// SetPaused saves the pause, so it survives restarts, then applies it.
func (controls *Controls) SetPaused(ctx context.Context, actor store.Actor, paused bool) error {
	if err := controls.Hub.SetModelWorkPaused(ctx, actor, paused); err != nil {
		return err
	}
	controls.Queue.SetPaused(paused)
	return nil
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
