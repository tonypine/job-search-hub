package modelqueue_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
)

// clock is a settable time for patience tests.
type clock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *clock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.now
}

func (clock *clock) Advance(by time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.now = clock.now.Add(by)
}

// waitAsync queues a ticket in the background and reports when it's granted.
func waitAsync(t *testing.T, queue *modelqueue.Queue, name string, ticket modelqueue.Ticket, order chan<- string) func() {
	t.Helper()
	releases := make(chan func(), 1)
	go func() {
		release, err := queue.Wait(context.Background(), ticket)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		order <- name
		releases <- release
	}()
	waitUntil(t, func() bool { return queued(queue, ticket.Kind) || running(queue, ticket.Kind) })
	return func() { (<-releases)() }
}

func queued(queue *modelqueue.Queue, kind string) bool {
	for _, ticket := range queue.Status().Waiting {
		if ticket.Kind == kind {
			return true
		}
	}
	return false
}

func running(queue *modelqueue.Queue, kind string) bool {
	status := queue.Status()
	return status.Running != nil && status.Running.Kind == kind
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	for range 200 {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func receive(t *testing.T, order <-chan string) string {
	t.Helper()
	select {
	case name := <-order:
		return name
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was granted")
		return ""
	}
}

func TestCallsRunOneAtATimeByPriority(t *testing.T) {
	queue := modelqueue.New(false, modelqueue.Settings{})
	order := make(chan string, 4)
	releaseFirst := waitAsync(t, queue, "facts 1", modelqueue.Ticket{Kind: "facts 1", Model: "big", Priority: modelqueue.PriorityBackground}, order)
	if got := receive(t, order); got != "facts 1" {
		t.Fatalf("first = %s", got)
	}
	releaseFacts := waitAsync(t, queue, "facts 2", modelqueue.Ticket{Kind: "facts 2", Model: "big", Priority: modelqueue.PriorityBackground}, order)
	releaseMail := waitAsync(t, queue, "mail", modelqueue.Ticket{Kind: "mail", Model: "big", Priority: modelqueue.PrioritySorting}, order)
	releaseDispatched := waitAsync(t, queue, "dispatched", modelqueue.Ticket{Kind: "dispatched", Model: "big", Priority: modelqueue.PriorityDispatched}, order)

	status := queue.Status()
	if status.Running == nil || status.Running.Kind != "facts 1" || len(status.Waiting) != 3 || status.Waiting[0].Kind != "dispatched" || status.Waiting[2].Kind != "facts 2" {
		t.Fatalf("status = %+v", status)
	}
	releaseFirst()
	if got := receive(t, order); got != "dispatched" {
		t.Fatalf("after the first: %s", got)
	}
	releaseDispatched()
	if got := receive(t, order); got != "mail" {
		t.Fatalf("then: %s", got)
	}
	releaseMail()
	if got := receive(t, order); got != "facts 2" {
		t.Fatalf("last: %s", got)
	}
	releaseFacts()
}

func TestTheLoadedModelKeepsItsTurnUntilAnotherCallRunsOutOfPatience(t *testing.T) {
	now := &clock{now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
	loaded := atomic.Value{}
	loaded.Store("big")
	queue := modelqueue.New(false, modelqueue.Settings{Now: now.Now, GetLoadedModel: func() string { return loaded.Load().(string) }})
	order := make(chan string, 8)

	releaseFacts := waitAsync(t, queue, "facts 1", modelqueue.Ticket{Kind: "facts 1", Model: "big", Priority: modelqueue.PriorityBackground}, order)
	receive(t, order)
	releaseMail := waitAsync(t, queue, "mail", modelqueue.Ticket{Kind: "mail", Model: "small", Priority: modelqueue.PrioritySorting}, order)
	releaseMoreFacts := waitAsync(t, queue, "facts 2", modelqueue.Ticket{Kind: "facts 2", Model: "big", Priority: modelqueue.PriorityBackground}, order)

	releaseFacts()
	if got := receive(t, order); got != "facts 2" {
		t.Fatalf("with mail waiting under its patience, %s ran; want the loaded model's facts", got)
	}
	now.Advance(5 * time.Minute)
	releaseMoreFacts()
	if got := receive(t, order); got != "mail" {
		t.Fatalf("after 5 minutes, %s ran; want the mail", got)
	}
	releaseMail()
}

func TestAPausedQueueRunsOnlyDispatchedCallsAndUnloads(t *testing.T) {
	unloads := atomic.Int32{}
	queue := modelqueue.New(true, modelqueue.Settings{UnloadModel: func() { unloads.Add(1) }})
	order := make(chan string, 4)
	releaseFacts := waitAsync(t, queue, "facts", modelqueue.Ticket{Kind: "facts", Model: "big", Priority: modelqueue.PriorityBackground}, order)
	releaseDispatched := waitAsync(t, queue, "dispatched", modelqueue.Ticket{Kind: "dispatched", Model: "big", Priority: modelqueue.PriorityDispatched}, order)
	if got := receive(t, order); got != "dispatched" {
		t.Fatalf("paused: %s ran", got)
	}
	releaseDispatched()
	select {
	case got := <-order:
		t.Fatalf("paused, yet %s ran", got)
	case <-time.After(100 * time.Millisecond):
	}
	waitUntil(t, func() bool { return unloads.Load() > 0 })

	queue.SetPaused(false)
	if got := receive(t, order); got != "facts" {
		t.Fatalf("after resuming: %s", got)
	}
	releaseFacts()
	queue.SetPaused(true)
	if !queue.Status().Paused {
		t.Fatal("still running after the pause")
	}
}

func TestACallThatStopsWaitingLeavesTheQueue(t *testing.T) {
	queue := modelqueue.New(true, modelqueue.Settings{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := queue.Wait(ctx, modelqueue.Ticket{Kind: "facts", Priority: modelqueue.PriorityBackground}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if status := queue.Status(); len(status.Waiting) != 0 || status.Running != nil {
		t.Fatalf("status = %+v", status)
	}
}

func TestAPriorityTravelsInTheContext(t *testing.T) {
	ctx := modelqueue.WithPriority(context.Background(), modelqueue.PriorityDispatched)
	if modelqueue.GetPriority(ctx, modelqueue.PriorityBackground) != modelqueue.PriorityDispatched ||
		modelqueue.GetPriority(context.Background(), modelqueue.PrioritySorting) != modelqueue.PrioritySorting {
		t.Fatal("priority lost")
	}
}
