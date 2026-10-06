package api

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// drainRetryAfter is how many seconds a start refused by a drain waits
// before trying again: about the restart the drain comes before.
const drainRetryAfter = 30

type drainResponse struct {
	Draining bool       `json:"draining"`
	Since    *time.Time `json:"since,omitempty"`
	// Running is the work still running, the oldest first.
	Running []runningWork `json:"running"`
}

type runningWork struct {
	drain.Work
	AgeSeconds int `json:"age_seconds"`
}

// RegisterDrainRoutes adds the owner-only routes that start a drain before a
// restart (POST), list what still runs (GET) and cancel it (DELETE). Asking
// what still runs keeps the drain going: left alone, it cancels itself.
func RegisterDrainRoutes(routes *http.ServeMux, hub *store.Store, drainer *drain.Drain, requireOwner func(http.Handler) http.Handler) {
	writeStatus := func(w http.ResponseWriter, r *http.Request) {
		running := drainer.Running()
		agentRuns, err := hub.ListRunningAgentRuns(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		for _, run := range agentRuns {
			running = append(running, drain.Work{Type: drain.TypeAgentRun, Kind: run.Kind, Subject: run.Input, StartedAt: run.StartedAt})
		}
		slices.SortStableFunc(running, func(left, right drain.Work) int { return left.StartedAt.Compare(right.StartedAt) })
		response := drainResponse{Draining: drainer.Draining(), Running: make([]runningWork, 0, len(running))}
		if since := drainer.Since(); !since.IsZero() {
			response.Since = &since
		}
		now := time.Now()
		for _, work := range running {
			response.Running = append(response.Running, runningWork{Work: work, AgeSeconds: int(now.Sub(work.StartedAt).Seconds())})
		}
		writeJSON(w, http.StatusOK, response)
	}
	routes.Handle("POST /v1/drain", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		drainer.Start()
		writeStatus(w, r)
	})))
	routes.Handle("GET /v1/drain", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		drainer.Touch()
		writeStatus(w, r)
	})))
	routes.Handle("DELETE /v1/drain", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		drainer.Cancel()
		writeStatus(w, r)
	})))
}

// writeDrainingOrContinue answers 503, with when to try again, while the
// hub drains, and says whether the caller may go on.
func writeDrainingOrContinue(w http.ResponseWriter, drainer *drain.Drain) bool {
	if !drainer.Draining() {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(drainRetryAfter))
	writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: drain.ErrDraining.Error()})
	return false
}
