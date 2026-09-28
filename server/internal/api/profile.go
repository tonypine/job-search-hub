package api

import (
	"encoding/json"
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type saveProfileRequest struct {
	Body string `json:"body"`
}

// RegisterProfileRoutes adds the owner-only routes for reading and saving
// the owner profile.
func RegisterProfileRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/profile", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		profile, err := hub.GetOwnerProfile(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, profile)
	})))

	routes.Handle("PUT /v1/profile", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request saveProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		profile, err := hub.SaveOwnerProfile(r.Context(), store.Actor{Kind: store.ActorOwner}, request.Body)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, profile)
	})))
}
