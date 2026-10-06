// Package drain lets the hub stop starting work before a restart. While it
// drains, background passes skip, the model queue grants no new turn,
// `claude -p` runs are refused and agent runs can't start; the work already
// running finishes, and is listed until it does.
package drain

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// ErrDraining refuses work that would start while the hub drains.
var ErrDraining = errors.New("the hub is draining for a restart")

// DefaultTimeout is how long the hub stays draining with nobody asking what
// still runs, before it cancels the drain by itself.
const DefaultTimeout = 10 * time.Minute

// Work types, as the list of running work names them.
const (
	TypeModelCall = "model_call"
	TypeClaudeRun = "claude_run"
	TypeAgentRun  = "agent_run"
)

// Work is something still running.
type Work struct {
	Type string `json:"type"`
	// Kind is the task: job_facts, job_brief, company_triage...
	Kind string `json:"kind"`
	// Subject is what it works on: a job's id, or an agent run's input.
	Subject   string    `json:"subject,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// Drain holds the draining flag and the work it tracks. A nil Drain never
// drains, tracks nothing and refuses nothing.
type Drain struct {
	timeout time.Duration
	now     func() time.Time

	mutex    sync.Mutex
	draining bool
	since    time.Time
	timer    *time.Timer
	// generation counts the timer's arming, so a timer that fires after
	// it was re-armed or cancelled does nothing.
	generation int
	listeners  []func(draining bool)
	listers    []func() []Work
	tracked    map[*Work]struct{}
	// changed is closed and replaced when tracked work ends.
	changed chan struct{}
}

// New returns a Drain that cancels itself when left draining for timeout
// without a Touch.
func New(timeout time.Duration) *Drain {
	return &Drain{timeout: timeout, now: time.Now, tracked: map[*Work]struct{}{}, changed: make(chan struct{})}
}

// OnChange tells listener when the hub starts or stops draining.
func (drain *Drain) OnChange(listener func(draining bool)) {
	drain.mutex.Lock()
	defer drain.mutex.Unlock()
	drain.listeners = append(drain.listeners, listener)
}

// AddLister adds running work the Drain doesn't track itself, such as the
// model queue's running call, to what Running lists.
func (drain *Drain) AddLister(lister func() []Work) {
	drain.mutex.Lock()
	defer drain.mutex.Unlock()
	drain.listers = append(drain.listers, lister)
}

// Start drains, or, already draining, re-arms the timeout.
func (drain *Drain) Start() {
	drain.mutex.Lock()
	started := !drain.draining
	if started {
		drain.draining = true
		drain.since = drain.now()
	}
	drain.armTimer()
	listeners := slices.Clone(drain.listeners)
	drain.mutex.Unlock()
	if started {
		slog.Info("the hub is draining")
		for _, listener := range listeners {
			listener(true)
		}
	}
}

// Touch re-arms the timeout of a drain someone is still watching.
func (drain *Drain) Touch() {
	drain.mutex.Lock()
	defer drain.mutex.Unlock()
	if drain.draining {
		drain.armTimer()
	}
}

// Cancel stops draining, so work starts again.
func (drain *Drain) Cancel() {
	drain.cancel(-1, "cancelled")
}

// cancel stops draining, unless generation is a timer's arming that was
// re-armed since; -1 always stops.
func (drain *Drain) cancel(generation int, reason string) {
	drain.mutex.Lock()
	if !drain.draining || (generation >= 0 && generation != drain.generation) {
		drain.mutex.Unlock()
		return
	}
	drain.draining = false
	drain.since = time.Time{}
	drain.generation++
	if drain.timer != nil {
		drain.timer.Stop()
		drain.timer = nil
	}
	listeners := slices.Clone(drain.listeners)
	drain.mutex.Unlock()
	slog.Info("the hub stopped draining", "reason", reason)
	for _, listener := range listeners {
		listener(false)
	}
}

// armTimer runs with the mutex held.
func (drain *Drain) armTimer() {
	drain.generation++
	generation := drain.generation
	if drain.timer != nil {
		drain.timer.Stop()
	}
	drain.timer = time.AfterFunc(drain.timeout, func() { drain.cancel(generation, "nobody asked what still runs for "+drain.timeout.String()) })
}

// Draining says whether the hub drains.
func (drain *Drain) Draining() bool {
	if drain == nil {
		return false
	}
	drain.mutex.Lock()
	defer drain.mutex.Unlock()
	return drain.draining
}

// Since is when the drain started, zero when the hub doesn't drain.
func (drain *Drain) Since() time.Time {
	if drain == nil {
		return time.Time{}
	}
	drain.mutex.Lock()
	defer drain.mutex.Unlock()
	return drain.since
}

// Begin tracks work about to start, or refuses it with ErrDraining. The
// work ends with the returned function.
func (drain *Drain) Begin(workType, kind, subject string) (end func(), err error) {
	if drain == nil {
		return func() {}, nil
	}
	drain.mutex.Lock()
	defer drain.mutex.Unlock()
	if drain.draining {
		return nil, ErrDraining
	}
	work := &Work{Type: workType, Kind: kind, Subject: subject, StartedAt: drain.now()}
	drain.tracked[work] = struct{}{}
	return sync.OnceFunc(func() {
		drain.mutex.Lock()
		defer drain.mutex.Unlock()
		delete(drain.tracked, work)
		close(drain.changed)
		drain.changed = make(chan struct{})
	}), nil
}

// Running lists the work running in the hub, the oldest first.
func (drain *Drain) Running() []Work {
	if drain == nil {
		return []Work{}
	}
	drain.mutex.Lock()
	running := make([]Work, 0, len(drain.tracked))
	for work := range drain.tracked {
		running = append(running, *work)
	}
	listers := slices.Clone(drain.listers)
	drain.mutex.Unlock()
	for _, lister := range listers {
		running = append(running, lister()...)
	}
	slices.SortStableFunc(running, func(left, right Work) int { return left.StartedAt.Compare(right.StartedAt) })
	return running
}

// idlePoll is how often WaitIdle checks the work it doesn't track itself.
const idlePoll = 100 * time.Millisecond

// WaitIdle waits until no work runs, and says whether none does: false
// when ctx ended first.
func (drain *Drain) WaitIdle(ctx context.Context) bool {
	if drain == nil {
		return true
	}
	ticker := time.NewTicker(idlePoll)
	defer ticker.Stop()
	for {
		drain.mutex.Lock()
		changed := drain.changed
		drain.mutex.Unlock()
		if len(drain.Running()) == 0 {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-changed:
		case <-ticker.C:
		}
	}
}

type contextKey struct{}

// NewContext carries drain to the background passes run with ctx.
func NewContext(ctx context.Context, drain *Drain) context.Context {
	return context.WithValue(ctx, contextKey{}, drain)
}

// IsDraining says whether the hub carried by ctx drains, so a background
// pass skips. A ctx with no Drain never drains.
func IsDraining(ctx context.Context) bool {
	drain, _ := ctx.Value(contextKey{}).(*Drain)
	return drain.Draining()
}
