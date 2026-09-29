package api

import (
	"net/http"
	"strconv"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type taskRunsResponse struct {
	Runs []store.TaskRun `json:"runs"`
}

// RegisterTaskRunRoutes adds the owner-only route that lists the requests
// the hub made to models.
func RegisterTaskRunRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/task-runs", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		runs, err := hub.ListTaskRuns(r.Context(), store.TaskRunFilter{
			Kind: r.URL.Query().Get("kind"), Outcome: r.URL.Query().Get("outcome"), Limit: limit,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, taskRunsResponse{Runs: runs})
	})))
}
