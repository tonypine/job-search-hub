package api

import (
	"errors"
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// RegisterInterviewPackRoutes adds the owner-only route that serves a
// pursued job's interview pack.
func RegisterInterviewPackRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/jobs/{id}/interview-pack", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		pack, err := hub.GetInterviewPack(r.Context(), id)
		if errors.Is(err, store.ErrInterviewPackNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "the job has no interview pack yet"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, pack)
	})))
}
