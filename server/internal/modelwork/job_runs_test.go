package modelwork

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestAJobsRunsStartStageByStage(t *testing.T) {
	controls := &Controls{}
	jobID := uuid.New()
	stages := [][]string{{"facts"}, {"brief"}, {"full", "cv", "pack"}, {"screen"}}
	type started struct {
		kind string
		done chan struct{}
	}
	starts := make(chan started)
	finished := controls.runStages(jobID, stages, func(kind string) (<-chan struct{}, error) {
		if kind == "pack" {
			return nil, CannotRunError{Reason: "once you pursue the job"}
		}
		done := make(chan struct{})
		starts <- started{kind: kind, done: done}
		return done, nil
	})
	for _, kind := range []string{"facts", "brief", "full", "cv", "pack", "screen"} {
		if !controls.isQueued(startKey{kind: kind, subjectID: jobID}) {
			t.Errorf("%s isn't queued before its stage", kind)
		}
	}
	if controls.isQueued(startKey{kind: "facts", subjectID: uuid.New()}) {
		t.Error("another job's facts are queued")
	}

	expectStart := func(want string) started {
		t.Helper()
		select {
		case got := <-starts:
			if got.kind != want {
				t.Fatalf("started %s, want %s", got.kind, want)
			}
			return got
		case <-time.After(5 * time.Second):
			t.Fatalf("%s never started", want)
		}
		return started{}
	}
	expectNoStart := func() {
		t.Helper()
		select {
		case got := <-starts:
			t.Fatalf("%s started before the stage before it ended", got.kind)
		case <-time.After(50 * time.Millisecond):
		}
	}

	facts := expectStart("facts")
	expectNoStart()
	close(facts.done)
	brief := expectStart("brief")
	expectNoStart()
	close(brief.done)
	full := expectStart("full")
	cv := expectStart("cv")
	close(full.done)
	expectNoStart()
	close(cv.done)
	screen := expectStart("screen")
	close(screen.done)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the stages never finished")
	}
	for _, kind := range []string{"facts", "brief", "full", "cv", "pack", "screen"} {
		if controls.isQueued(startKey{kind: kind, subjectID: jobID}) {
			t.Errorf("%s is still queued once the stages finished", kind)
		}
	}
}

func TestAJobsRunsGoOnPastAKindThatFailsToStart(t *testing.T) {
	controls := &Controls{}
	var startedKinds []string
	finished := controls.runStages(uuid.New(), [][]string{{"facts"}, {"brief"}}, func(kind string) (<-chan struct{}, error) {
		startedKinds = append(startedKinds, kind)
		if kind == "facts" {
			return nil, errors.New("no model is routed for it yet")
		}
		done := make(chan struct{})
		close(done)
		return done, nil
	})
	<-finished
	if !slices.Equal(startedKinds, []string{"facts", "brief"}) {
		t.Fatalf("started = %v", startedKinds)
	}
}

func TestAJobsRunsAreItsStartableJobKindsButItsFix(t *testing.T) {
	var kinds []string
	for _, stage := range JobRunStages {
		for _, kind := range stage {
			if _, startable := starters[kind]; !startable {
				t.Errorf("%s isn't startable", kind)
			}
			kinds = append(kinds, kind)
		}
	}
	for _, kind := range store.RunKinds {
		if _, startable := starters[kind.ID]; startable && kind.SubjectType == store.RunSubjectJob && kind.ID != store.AgentRunKindJobFix && !slices.Contains(kinds, kind.ID) {
			t.Errorf("%s isn't started for a job just added", kind.ID)
		}
	}
	if slices.Contains(kinds, store.AgentRunKindJobFix) {
		t.Error("a fix of the job's details is started")
	}
}
