package modelwork

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// JobRunStages are the kinds started for a job just added, stage by stage.
// A stage starts once every run of the one before it ended, since its kinds
// read what those made: a brief reads the job's facts, and a recruiter
// screen the tailored CV. A fix of the job's details isn't among them.
var JobRunStages = [][]string{
	{store.AgentPromptKindJobFacts},
	{store.AgentPromptKindJobBrief},
	{store.RunKindJobBriefFull, store.AgentPromptKindJobCV, store.AgentPromptKindInterviewPrep},
	{store.AgentPromptKindRecruiterScreen},
}

// StartJobRuns starts every run of a job just added, stage by stage, as the
// owner would from its Run menu: whatever each kind's pause, and on its
// kind's model. It returns at once. Until its stage comes, a kind is listed
// queued for the job. A kind that can't run when its stage comes, as a
// tailored CV with no base CV saved or an interview pack for a job not
// pursued, isn't started and keeps its reason.
func (controls *Controls) StartJobRuns(ctx context.Context, actor store.Actor, jobID uuid.UUID) {
	runCtx := context.WithoutCancel(ctx)
	controls.runStages(jobID, JobRunStages, func(kind string) (<-chan struct{}, error) {
		_, done, err := controls.start(runCtx, actor, StartRequest{Kind: kind, SubjectType: store.RunSubjectJob, SubjectID: jobID})
		return done, err
	})
}

// runStages lists every kind of the stages queued for the job, then starts
// them in the background, each stage once the runs of the one before ended.
// start returns a channel that closes once its run ended, nil for a run it
// doesn't follow.
func (controls *Controls) runStages(jobID uuid.UUID, stages [][]string, start func(kind string) (<-chan struct{}, error)) <-chan struct{} {
	for _, stage := range stages {
		for _, kind := range stage {
			controls.setQueued(startKey{kind: kind, subjectID: jobID}, true)
		}
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for _, stage := range stages {
			var running []<-chan struct{}
			for _, kind := range stage {
				done, err := start(kind)
				// The start lists its run running before it returns, so the
				// kind is never listed neither queued nor running.
				controls.setQueued(startKey{kind: kind, subjectID: jobID}, false)
				if err != nil {
					slog.Info("a run of a job just added wasn't started", "kind", kind, "job", jobID, "reason", err)
					continue
				}
				if done != nil {
					running = append(running, done)
				}
			}
			for _, done := range running {
				<-done
			}
		}
	}()
	return finished
}

// setQueued lists the kind queued for the subject, or no longer.
func (controls *Controls) setQueued(key startKey, isQueued bool) {
	controls.starting.Lock()
	defer controls.starting.Unlock()
	if !isQueued {
		delete(controls.queued, key)
		return
	}
	if controls.queued == nil {
		controls.queued = map[startKey]bool{}
	}
	controls.queued[key] = true
}

// isQueued says the kind waits for its stage to start for the subject.
func (controls *Controls) isQueued(key startKey) bool {
	controls.starting.Lock()
	defer controls.starting.Unlock()
	return controls.queued[key]
}
