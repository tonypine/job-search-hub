package api

import (
	"encoding/json"
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// RegisterJobCriteriaRoutes adds the owner-only routes for reading and
// saving the job criteria.
func RegisterJobCriteriaRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/job-criteria", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saved, err := hub.GetJobCriteria(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, saved)
	})))

	routes.Handle("PUT /v1/job-criteria", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var criteria store.JobCriteria
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&criteria); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be the criteria as JSON: " + err.Error()})
			return
		}
		saved, err := hub.SaveJobCriteria(r.Context(), store.Actor{Kind: store.ActorOwner}, criteria)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, saved)
	})))
}
