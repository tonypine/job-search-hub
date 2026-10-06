package drain_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/drain"
)

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	for range 400 {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestNoWorkStartsWhileDrainingAndRunningWorkIsListedUntilItEnds(t *testing.T) {
	hub := drain.New(time.Hour)
	end, err := hub.Begin(drain.TypeClaudeRun, "job_brief", "job-1")
	if err != nil {
		t.Fatalf("begin before draining: %v", err)
	}

	hub.Start()
	if !hub.Draining() || hub.Since().IsZero() {
		t.Fatalf("draining = %v since %v", hub.Draining(), hub.Since())
	}
	if _, err := hub.Begin(drain.TypeClaudeRun, "job_brief", "job-2"); !errors.Is(err, drain.ErrDraining) {
		t.Fatalf("begin while draining: %v, want ErrDraining", err)
	}
	running := hub.Running()
	if len(running) != 1 || running[0].Type != drain.TypeClaudeRun || running[0].Kind != "job_brief" || running[0].Subject != "job-1" || running[0].StartedAt.IsZero() {
		t.Fatalf("running = %+v", running)
	}

	end()
	end()
	if running := hub.Running(); len(running) != 0 {
		t.Fatalf("running after the end = %+v", running)
	}
	hub.Cancel()
	if hub.Draining() {
		t.Fatal("still draining after Cancel")
	}
	end, err = hub.Begin(drain.TypeClaudeRun, "job_brief", "job-2")
	if err != nil {
		t.Fatalf("begin after the drain: %v", err)
	}
	end()
}

func TestTheRunningWorkListsAddedWorkOldestFirst(t *testing.T) {
	hub := drain.New(time.Hour)
	older := drain.Work{Type: drain.TypeModelCall, Kind: "job_facts", StartedAt: time.Now().Add(-time.Minute)}
	hub.AddLister(func() []drain.Work { return []drain.Work{older} })
	end, _ := hub.Begin(drain.TypeClaudeRun, "cv_draft", "")
	defer end()

	running := hub.Running()
	if len(running) != 2 || running[0].Type != drain.TypeModelCall || running[1].Type != drain.TypeClaudeRun {
		t.Fatalf("running = %+v", running)
	}
}

func TestDrainingCancelsItselfWhenNobodyAsksForTheTimeout(t *testing.T) {
	hub := drain.New(50 * time.Millisecond)
	var mutex sync.Mutex
	var changes []bool
	hub.OnChange(func(draining bool) {
		mutex.Lock()
		defer mutex.Unlock()
		changes = append(changes, draining)
	})

	hub.Start()
	waitUntil(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return len(changes) == 2
	})
	mutex.Lock()
	defer mutex.Unlock()
	if hub.Draining() || !changes[0] || changes[1] {
		t.Fatalf("changes = %v, want [true false]", changes)
	}
}

func TestAskingWhatStillRunsKeepsTheDrainGoing(t *testing.T) {
	hub := drain.New(300 * time.Millisecond)
	hub.Start()
	for range 4 {
		time.Sleep(100 * time.Millisecond)
		hub.Touch()
	}
	if !hub.Draining() {
		t.Fatal("the drain cancelled itself while being asked")
	}
	waitUntil(t, func() bool { return !hub.Draining() })
}

func TestARestartedDrainOutlivesTheFirstOnesTimeout(t *testing.T) {
	hub := drain.New(300 * time.Millisecond)
	hub.Start()
	time.Sleep(200 * time.Millisecond)
	hub.Cancel()
	hub.Start()
	time.Sleep(200 * time.Millisecond)
	if !hub.Draining() {
		t.Fatal("the first drain's timeout cancelled the second")
	}
}

func TestWaitIdleWaitsForTheRunningWork(t *testing.T) {
	hub := drain.New(time.Hour)
	end, _ := hub.Begin(drain.TypeClaudeRun, "job_brief", "job-1")
	modelCallRunning := true
	var mutex sync.Mutex
	hub.AddLister(func() []drain.Work {
		mutex.Lock()
		defer mutex.Unlock()
		if modelCallRunning {
			return []drain.Work{{Type: drain.TypeModelCall, Kind: "job_facts"}}
		}
		return nil
	})
	hub.Start()

	idle := make(chan bool, 1)
	go func() { idle <- hub.WaitIdle(context.Background()) }()
	end()
	select {
	case <-idle:
		t.Fatal("WaitIdle returned while a model call still runs")
	case <-time.After(150 * time.Millisecond):
	}
	mutex.Lock()
	modelCallRunning = false
	mutex.Unlock()
	select {
	case got := <-idle:
		if !got {
			t.Fatal("WaitIdle = false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitIdle didn't return once the work ended")
	}
}

func TestWaitIdleGivesUpWithItsContext(t *testing.T) {
	hub := drain.New(time.Hour)
	end, _ := hub.Begin(drain.TypeClaudeRun, "job_brief", "job-1")
	defer end()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if hub.WaitIdle(ctx) {
		t.Fatal("WaitIdle = true with work running")
	}
}

func TestANilDrainNeverDrains(t *testing.T) {
	var hub *drain.Drain
	end, err := hub.Begin(drain.TypeClaudeRun, "job_brief", "")
	if err != nil || hub.Draining() || len(hub.Running()) != 0 || !hub.WaitIdle(context.Background()) {
		t.Fatalf("nil drain: err %v, draining %v", err, hub.Draining())
	}
	end()
}

func TestAContextCarriesTheDrainToBackgroundPasses(t *testing.T) {
	hub := drain.New(time.Hour)
	ctx := drain.NewContext(context.Background(), hub)
	if drain.IsDraining(ctx) || drain.IsDraining(context.Background()) {
		t.Fatal("draining before Start")
	}
	hub.Start()
	if !drain.IsDraining(ctx) || drain.IsDraining(context.Background()) {
		t.Fatal("the context doesn't tell the drain")
	}
}
