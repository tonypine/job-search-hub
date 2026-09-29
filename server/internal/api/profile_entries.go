package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type profileEntriesResponse struct {
	Entries []store.ProfileEntry `json:"entries"`
}

type confirmProfileEntriesRequest struct {
	IDs []uuid.UUID `json:"ids"`
}

// RegisterProfileEntryRoutes adds the owner-only routes for the knowledge base
// of the owner's experience.
func RegisterProfileEntryRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("GET /v1/profile/entries", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := store.ProfileEntryFilter{Kind: r.URL.Query().Get("kind")}
		if raw := r.URL.Query().Get("confirmed"); raw != "" {
			confirmed, err := strconv.ParseBool(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "confirmed must be true or false"})
				return
			}
			filter.Confirmed = &confirmed
		}
		entries, err := hub.ListProfileEntries(r.Context(), filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, profileEntriesResponse{Entries: entries})
	})))

	save := func(w http.ResponseWriter, r *http.Request, id *uuid.UUID, successStatus int) {
		var input store.ProfileEntryInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		saved, err := hub.SaveProfileEntry(r.Context(), owner, id, input)
		switch {
		case errors.Is(err, store.ErrProfileEntryNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, successStatus, saved)
		}
	}
	routes.Handle("POST /v1/profile/entries", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		save(w, r, nil, http.StatusCreated)
	})))
	routes.Handle("PUT /v1/profile/entries/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		save(w, r, &id, http.StatusOK)
	})))

	routes.Handle("POST /v1/profile/entries/confirm", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request confirmProfileEntriesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		confirmed, err := hub.ConfirmProfileEntries(r.Context(), owner, request.IDs)
		switch {
		case errors.Is(err, store.ErrProfileEntryNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, profileEntriesResponse{Entries: confirmed})
		}
	})))

	routes.Handle("DELETE /v1/profile/entries/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		err = hub.DeleteProfileEntry(r.Context(), owner, id)
		switch {
		case errors.Is(err, store.ErrProfileEntryNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})))
}
