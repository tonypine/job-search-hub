package jobfacts

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// firstJobRetryDelay is how long a job the model failed on waits before
	// the background reads it again; each failure in a row doubles it, up to
	// maximumJobRetryDelay.
	firstJobRetryDelay   = 15 * time.Minute
	maximumJobRetryDelay = 6 * time.Hour
	// failuresBeforePause failures in a row, whichever jobs they are on, stop
	// the pass and pause the background, as when the CLI fails every call:
	// first for firstPauseDelay, doubling up to maximumPauseDelay.
	failuresBeforePause = 3
	firstPauseDelay     = 5 * time.Minute
	maximumPauseDelay   = time.Hour
)

// backoff keeps the background from retrying failures in a tight loop: a job
// the model failed on waits before its next try, and a run of failures
// pauses every job. A success on any job ends the pause.
type backoff struct {
	mutex sync.Mutex
	jobs  map[uuid.UUID]jobRetry
	// failuresInARow counts the failures since the last success; pauses counts
	// the pauses they have caused.
	failuresInARow int
	pauses         int
	pausedUntil    time.Time
}

type jobRetry struct {
	failures int
	at       time.Time
}

// pausedUntilAt is when the pause ends, or zero when there is none at now.
func (b *backoff) pausedUntilAt(now time.Time) time.Time {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if now.Before(b.pausedUntil) {
		return b.pausedUntil
	}
	return time.Time{}
}

// isWaiting is whether the job failed recently and its retry isn't due.
func (b *backoff) isWaiting(jobID uuid.UUID, now time.Time) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	retry, ok := b.jobs[jobID]
	return ok && now.Before(retry.at)
}

// succeeded forgets the job's failures, and ends the run of failures.
func (b *backoff) succeeded(jobID uuid.UUID) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	delete(b.jobs, jobID)
	b.failuresInARow, b.pauses, b.pausedUntil = 0, 0, time.Time{}
}

// failed puts the job's next try off, and returns when the background pause
// it starts ends, or zero when it starts none.
func (b *backoff) failed(jobID uuid.UUID, now time.Time) time.Time {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.jobs == nil {
		b.jobs = map[uuid.UUID]jobRetry{}
	}
	retry := b.jobs[jobID]
	retry.failures++
	retry.at = now.Add(doubled(firstJobRetryDelay, retry.failures, maximumJobRetryDelay))
	b.jobs[jobID] = retry

	b.failuresInARow++
	if b.failuresInARow%failuresBeforePause != 0 {
		return time.Time{}
	}
	b.pauses++
	b.pausedUntil = now.Add(doubled(firstPauseDelay, b.pauses, maximumPauseDelay))
	return b.pausedUntil
}

// doubled is first doubled for each time after the first, at most maximum.
func doubled(first time.Duration, times int, maximum time.Duration) time.Duration {
	delay := first
	for range times - 1 {
		if delay >= maximum {
			break
		}
		delay *= 2
	}
	return min(delay, maximum)
}
