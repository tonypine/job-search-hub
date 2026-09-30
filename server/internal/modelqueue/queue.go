// Package modelqueue decides which model call runs next: one at a time, the
// owner's own runs first, then sorting, then background work, preferring the
// model already loaded so the runtime swaps as little as it can.
package modelqueue

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Priority orders waiting calls; a lower one runs sooner.
type Priority int

const (
	// PriorityDispatched is a run the owner started, such as "Read facts
	// now". It runs next, even while background work is paused.
	PriorityDispatched Priority = iota
	// PrioritySorting is mail and conversation sorting, which the owner
	// notices when it lags.
	PrioritySorting
	// PriorityBackground is work nobody waits on, such as reading job facts.
	PriorityBackground
)

func (priority Priority) String() string {
	switch priority {
	case PriorityDispatched:
		return "dispatched"
	case PrioritySorting:
		return "sorting"
	default:
		return "background"
	}
}

// DefaultPatience is how long a call for another model waits before the
// queue swaps models for it, by priority.
var DefaultPatience = map[Priority]time.Duration{
	PriorityDispatched: 0,
	PrioritySorting:    5 * time.Minute,
	PriorityBackground: 15 * time.Minute,
}

// Ticket describes a call waiting for its turn.
type Ticket struct {
	Kind      string
	SubjectID *uuid.UUID
	Model     string
	Priority  Priority
}

// TicketStatus is a ticket as the status shows it.
type TicketStatus struct {
	Kind      string     `json:"kind"`
	SubjectID *uuid.UUID `json:"subject_id,omitempty"`
	Model     string     `json:"model"`
	Priority  string     `json:"priority"`
	// Since is when it started waiting, or started running.
	Since time.Time `json:"since"`
}

// Status is what the queue is doing.
type Status struct {
	Paused  bool           `json:"paused"`
	Running *TicketStatus  `json:"running,omitempty"`
	Waiting []TicketStatus `json:"waiting"`
}

type queuedTicket struct {
	Ticket
	since   time.Time
	granted chan struct{}
}

// Settings wires the queue to the rest of the hub; every field is optional.
type Settings struct {
	// Patience overrides DefaultPatience.
	Patience map[Priority]time.Duration
	// GetLoadedModel names the model in memory, "" when none is.
	GetLoadedModel func() string
	// UnloadModel frees the model's memory; it runs once the queue is
	// paused and nothing runs.
	UnloadModel func()
	// Now stands in for time.Now in tests.
	Now func() time.Time
}

type Queue struct {
	settings Settings

	mutex   sync.Mutex
	paused  bool
	running *queuedTicket
	waiting []*queuedTicket
	// lastModel is the model of the last call granted, the loaded one when
	// the settings can't say.
	lastModel string
}

func New(paused bool, settings Settings) *Queue {
	if settings.Patience == nil {
		settings.Patience = DefaultPatience
	}
	if settings.Now == nil {
		settings.Now = time.Now
	}
	return &Queue{settings: settings, paused: paused}
}

// Wait blocks until the ticket's turn and returns the function that ends it.
func (queue *Queue) Wait(ctx context.Context, ticket Ticket) (release func(), err error) {
	waiting := &queuedTicket{Ticket: ticket, since: queue.settings.Now(), granted: make(chan struct{})}
	queue.mutex.Lock()
	queue.waiting = append(queue.waiting, waiting)
	queue.grantNext()
	queue.mutex.Unlock()

	select {
	case <-waiting.granted:
		return func() { queue.finish(waiting) }, nil
	case <-ctx.Done():
		queue.mutex.Lock()
		defer queue.mutex.Unlock()
		select {
		case <-waiting.granted: // granted as ctx ended: give the turn back
			queue.running = nil
			queue.grantNext()
		default:
			queue.waiting = slices.DeleteFunc(queue.waiting, func(other *queuedTicket) bool { return other == waiting })
		}
		return nil, ctx.Err()
	}
}

func (queue *Queue) finish(ticket *queuedTicket) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	if queue.running == ticket {
		queue.running = nil
		queue.grantNext()
	}
}

// SetPaused pauses or resumes the queue. While paused, background and
// sorting calls don't start (the running call finishes); dispatched calls
// still run. The caller keeps the pause across restarts.
func (queue *Queue) SetPaused(paused bool) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.paused = paused
	queue.grantNext()
}

// Status reports the pause, the running call and the waiting ones, in the
// order they would run.
func (queue *Queue) Status() Status {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	status := Status{Paused: queue.paused, Waiting: []TicketStatus{}}
	if queue.running != nil {
		running := convertTicketToStatus(queue.running)
		status.Running = &running
	}
	for _, ticket := range queue.getWaitingInOrder() {
		status.Waiting = append(status.Waiting, convertTicketToStatus(ticket))
	}
	return status
}

func convertTicketToStatus(ticket *queuedTicket) TicketStatus {
	return TicketStatus{Kind: ticket.Kind, SubjectID: ticket.SubjectID, Model: ticket.Model, Priority: ticket.Priority.String(), Since: ticket.since}
}

func (queue *Queue) getWaitingInOrder() []*queuedTicket {
	ordered := slices.Clone(queue.waiting)
	slices.SortStableFunc(ordered, func(left, right *queuedTicket) int {
		if left.Priority != right.Priority {
			return int(left.Priority - right.Priority)
		}
		return left.since.Compare(right.since)
	})
	return ordered
}

// grantNext gives the turn to the next call when none runs. It runs with the
// mutex held.
func (queue *Queue) grantNext() {
	if queue.running != nil {
		return
	}
	next := queue.chooseNext()
	if next == nil {
		if queue.paused && queue.settings.UnloadModel != nil {
			go queue.settings.UnloadModel()
		}
		return
	}
	queue.waiting = slices.DeleteFunc(queue.waiting, func(other *queuedTicket) bool { return other == next })
	next.since = queue.settings.Now()
	queue.running = next
	queue.lastModel = next.Model
	close(next.granted)
}

// chooseNext picks the first call by priority, then age. A call for a model
// other than the loaded one yields to a call for the loaded model until it
// has waited past its patience, so swaps come in batches.
func (queue *Queue) chooseNext() *queuedTicket {
	var eligible []*queuedTicket
	for _, ticket := range queue.getWaitingInOrder() {
		if !queue.paused || ticket.Priority == PriorityDispatched {
			eligible = append(eligible, ticket)
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	first := eligible[0]
	loaded := queue.lastModel
	if queue.settings.GetLoadedModel != nil {
		loaded = queue.settings.GetLoadedModel()
	}
	if loaded == "" || first.Model == loaded || queue.settings.Now().Sub(first.since) >= queue.settings.Patience[first.Priority] {
		return first
	}
	for _, ticket := range eligible[1:] {
		if ticket.Model == loaded {
			return ticket
		}
	}
	return first
}

type priorityKey struct{}

// WithPriority marks the calls made with ctx as priority.
func WithPriority(ctx context.Context, priority Priority) context.Context {
	return context.WithValue(ctx, priorityKey{}, priority)
}

// GetPriority returns ctx's priority, or fallback when it has none.
func GetPriority(ctx context.Context, fallback Priority) Priority {
	if priority, marked := ctx.Value(priorityKey{}).(Priority); marked {
		return priority
	}
	return fallback
}
