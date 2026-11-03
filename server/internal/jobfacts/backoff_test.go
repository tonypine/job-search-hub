package jobfacts

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAFailedJobWaitsLongerAfterEachFailureUntilItSucceeds(t *testing.T) {
	var jobs backoff
	job, start := uuid.New(), time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	jobs.failuresInARow = -100 // keeps the pause out of this test

	jobs.failed(job, start)
	if !jobs.isWaiting(job, start.Add(14*time.Minute)) || jobs.isWaiting(job, start.Add(15*time.Minute)) {
		t.Fatal("the first retry isn't 15 minutes after the failure")
	}
	jobs.failed(job, start.Add(15*time.Minute))
	if !jobs.isWaiting(job, start.Add(44*time.Minute)) || jobs.isWaiting(job, start.Add(45*time.Minute)) {
		t.Fatal("the second retry isn't 30 minutes after the second failure")
	}
	for range 10 {
		jobs.failed(job, start)
	}
	if !jobs.isWaiting(job, start.Add(6*time.Hour-time.Minute)) || jobs.isWaiting(job, start.Add(6*time.Hour)) {
		t.Fatal("the retry delay isn't capped at 6 hours")
	}
	if jobs.isWaiting(uuid.New(), start) {
		t.Fatal("a job that never failed waits")
	}
	jobs.succeeded(job)
	if jobs.isWaiting(job, start) {
		t.Fatal("a job that succeeded still waits")
	}
}

func TestFailuresInARowPauseTheBackgroundLongerEachTimeUntilASuccess(t *testing.T) {
	var jobs backoff
	start := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	failThree := func(at time.Time) time.Time {
		t.Helper()
		if until := jobs.failed(uuid.New(), at); !until.IsZero() {
			t.Fatalf("the first failure paused until %v", until)
		}
		if until := jobs.failed(uuid.New(), at); !until.IsZero() {
			t.Fatalf("the second failure paused until %v", until)
		}
		return jobs.failed(uuid.New(), at)
	}

	if until := failThree(start); !until.Equal(start.Add(5 * time.Minute)) {
		t.Fatalf("the first pause ends at %v", until)
	}
	if !jobs.pausedUntilAt(start.Add(4*time.Minute)).Equal(start.Add(5*time.Minute)) || !jobs.pausedUntilAt(start.Add(5*time.Minute)).IsZero() {
		t.Fatal("the pause doesn't end after 5 minutes")
	}
	wants := []time.Duration{10 * time.Minute, 20 * time.Minute, 40 * time.Minute, time.Hour, time.Hour}
	for _, want := range wants {
		if until := failThree(start); !until.Equal(start.Add(want)) {
			t.Fatalf("a pause ends at %v, want after %v", until, want)
		}
	}

	jobs.succeeded(uuid.New())
	if !jobs.pausedUntilAt(start).IsZero() {
		t.Fatal("a success doesn't end the pause")
	}
	if until := failThree(start); !until.Equal(start.Add(5 * time.Minute)) {
		t.Fatalf("after a success, the next pause ends at %v", until)
	}
}
