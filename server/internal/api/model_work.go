package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// RegisterModelWorkRoutes adds the owner-only routes that show the hub's
// model work, pause and resume it, and read one job's facts now.
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
			if err := controls.SetPaused(r.Context(), owner, paused); err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			writeStatus(w, r)
		})))
	}

	routes.Handle("POST /v1/jobs/{id}/facts/read", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		err = controls.DispatchJobFacts(r.Context(), id)
		switch {
		case errors.Is(err, store.ErrJobNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case errors.Is(err, modelwork.ErrNotSetUp):
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
		}
	})))
}
