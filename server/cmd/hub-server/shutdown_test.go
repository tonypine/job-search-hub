package main

import (
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/drain"
)

// startServing serves an empty handler until the test ends.
func startServing(t *testing.T) *http.Server {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func TestAStoppingServerDrainsAndWaitsForTheRunningWork(t *testing.T) {
	server := startServing(t)
	drainer := drain.New(time.Hour)
	end, err := drainer.Begin(drain.TypeClaudeRun, "job_brief", "job-1")
	if err != nil {
		t.Fatal(err)
	}
	var workStopped atomic.Bool
	done := make(chan struct{})
	go func() {
		shutDown(server, drainer, func() { workStopped.Store(true) }, 5*time.Second)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("shut down with a claude run still running")
	default:
	}
	if !drainer.Draining() || workStopped.Load() {
		t.Fatalf("draining = %v, work stopped = %v, want draining with the work going on", drainer.Draining(), workStopped.Load())
	}
	if _, err := drainer.Begin(drain.TypeClaudeRun, "job_brief", "job-2"); err == nil {
		t.Fatal("new work started while the server stopped")
	}

	end()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("didn't shut down once the work finished")
	}
	if !workStopped.Load() {
		t.Fatal("the workers weren't stopped")
	}
}

func TestAStoppingServerGivesUpOnWorkThatOutlastsTheWait(t *testing.T) {
	server := startServing(t)
	drainer := drain.New(time.Hour)
	end, _ := drainer.Begin(drain.TypeClaudeRun, "job_brief", "job-1")
	defer end()
	var workStopped atomic.Bool

	started := time.Now()
	shutDown(server, drainer, func() { workStopped.Store(true) }, 100*time.Millisecond)
	if waited := time.Since(started); waited < 100*time.Millisecond || waited > 2*time.Second {
		t.Fatalf("waited %v, want about the 100ms given", waited)
	}
	if !workStopped.Load() {
		t.Fatal("the workers weren't stopped")
	}
}
