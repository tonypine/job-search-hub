package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type modelProvidersResponse struct {
	Providers []store.ModelProvider `json:"providers"`
}

type taskRoutesResponse struct {
	Routes []store.TaskRoute `json:"routes"`
}

// RegisterModelRoutingRoutes adds the owner-only routes for the model servers
// the hub can call and the model each kind of task runs on. A provider's key
// can be set but is never served back.
func RegisterModelRoutingRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("GET /v1/model-providers", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providers, err := hub.ListModelProviders(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, modelProvidersResponse{Providers: providers})
	})))

	saveProvider := func(w http.ResponseWriter, r *http.Request, id *uuid.UUID, successStatus int) {
		var input store.ModelProviderInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		saved, err := hub.SaveModelProvider(r.Context(), owner, id, input)
		switch {
		case errors.Is(err, store.ErrModelProviderNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, successStatus, saved)
		}
	}
	routes.Handle("POST /v1/model-providers", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saveProvider(w, r, nil, http.StatusCreated)
	})))
	routes.Handle("PUT /v1/model-providers/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		saveProvider(w, r, &id, http.StatusOK)
	})))
	routes.Handle("DELETE /v1/model-providers/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		err = hub.DeleteModelProvider(r.Context(), owner, id)
		switch {
		case errors.Is(err, store.ErrModelProviderNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case errors.Is(err, store.ErrModelProviderInUse):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})))

	routes.Handle("GET /v1/task-routes", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		taskRoutes, err := hub.ListTaskRoutes(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, taskRoutesResponse{Routes: taskRoutes})
	})))
	routes.Handle("PUT /v1/task-routes/{kind}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input store.TaskRouteInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		saved, err := hub.SaveTaskRoute(r.Context(), owner, r.PathValue("kind"), input)
		switch {
		case errors.Is(err, store.ErrModelProviderNotFound):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "no such provider"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, saved)
		}
	})))
}
