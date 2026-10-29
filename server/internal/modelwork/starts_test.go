package modelwork

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestAStartedRunIsListedRunningUntilItEnds(t *testing.T) {
	controls := &Controls{}
	brief := startKey{kind: "job_brief_full", subjectID: uuid.New()}
	otherJob := startKey{kind: brief.kind, subjectID: uuid.New()}
	if running := controls.listRunningStarts(brief); running == nil || len(running) != 0 {
		t.Fatalf("running before any start = %#v, want none", running)
	}

	first, second, other := uuid.New(), uuid.New(), uuid.New()
	controls.beginStart(brief, first)
	controls.beginStart(brief, second)
	controls.beginStart(otherJob, other)
	if running := controls.listRunningStarts(brief); !slices.Equal(running, []uuid.UUID{first, second}) {
		t.Fatalf("running = %v, want %v then %v", running, first, second)
	}

	controls.endStart(brief, first)
	if running := controls.listRunningStarts(brief); !slices.Equal(running, []uuid.UUID{second}) {
		t.Fatalf("running once the first ended = %v, want %v", running, second)
	}
	controls.endStart(brief, second)
	if running := controls.listRunningStarts(brief); running == nil || len(running) != 0 {
		t.Fatalf("running once both ended = %#v, want none", running)
	}
	if running := controls.listRunningStarts(otherJob); !slices.Equal(running, []uuid.UUID{other}) {
		t.Fatalf("another job's running = %v, want %v", running, other)
	}
}
