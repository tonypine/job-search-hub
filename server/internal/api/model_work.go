package api

import (
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// RegisterModelWorkRoutes adds the owner-only routes that show the hub's
// model work, and pause and resume every kind of it.
func RegisterModelWorkRoutes(routes *http.ServeMux, controls *modelwork.Controls, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	writeStatus := func(w http.ResponseWriter, r *http.Request) {
		status, err := controls.GetStatus(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
	routes.Handle("GET /v1/model-work", requireOwner(http.HandlerFunc(writeStatus)))
	for path, paused := range map[string]bool{"/v1/model-work/pause": true, "/v1/model-work/resume": false} {
		routes.Handle("POST "+path, requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := controls.SetAllPaused(r.Context(), owner, paused); err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			writeStatus(w, r)
		})))
	}
}
