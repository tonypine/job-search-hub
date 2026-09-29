package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type addWarmPathRequest struct {
	Name             string `json:"name"`
	HowKnown         string `json:"how_known"`
	PreferredChannel string `json:"preferred_channel"`
	Note             string `json:"note"`
}

// RegisterWarmPathRoutes adds the owner-only routes for linking people the
// owner knows to the companies they can open doors at.
func RegisterWarmPathRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("POST /v1/companies/{id}/warm-paths", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		companyID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		var request addWarmPathRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		path, err := hub.AddWarmPath(r.Context(), owner, companyID, store.NewWarmPath{
			Name: request.Name, HowKnown: request.HowKnown, PreferredChannel: request.PreferredChannel, Note: request.Note,
		})
		switch {
		case errors.Is(err, store.ErrCompanyNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "no such company"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusCreated, path)
		}
	})))

	routes.Handle("DELETE /v1/companies/{id}/warm-paths/{contact_id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		companyID, companyErr := uuid.Parse(r.PathValue("id"))
		contactID, contactErr := uuid.Parse(r.PathValue("contact_id"))
		if companyErr != nil || contactErr != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		err := hub.RemoveWarmPath(r.Context(), owner, companyID, contactID)
		switch {
		case errors.Is(err, store.ErrWarmPathNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})))
}
