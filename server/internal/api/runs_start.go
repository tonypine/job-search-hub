package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type runnableKindsResponse struct {
	Kinds []modelwork.RunnableKind `json:"kinds"`
}

type startedRunResponse struct {
	// Queued is always true; the routes that started one kind answered only
	// it before they answered with the run.
	Queued bool                 `json:"queued"`
	Run    modelwork.StartedRun `json:"run"`
}

// RegisterRunStartRoutes adds the owner-only routes that list the kinds of
// run a job or a company can have, each with its model, its latest run for
// it and whether it can run now or lacks only a model, and that start one
// now, on its kind's model or one chosen for that run only. A started run
// goes ahead of background work whatever its kind's pause. Reading a job's
// facts now, its full brief and its tailored CV start the same way.
func RegisterRunStartRoutes(routes *http.ServeMux, controls *modelwork.Controls, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	for path, subjectType := range map[string]string{"/v1/jobs/{id}/runnable-kinds": store.RunSubjectJob, "/v1/companies/{id}/runnable-kinds": store.RunSubjectCompany} {
		routes.Handle("GET "+path, requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := parsePathIDOrWriteNotFound(w, r)
			if !ok {
				return
			}
			kinds, err := controls.ListRunnableKinds(r.Context(), subjectType, id)
			switch {
			case errors.Is(err, store.ErrJobNotFound), errors.Is(err, store.ErrCompanyNotFound):
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			case err != nil:
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			default:
				writeJSON(w, http.StatusOK, runnableKindsResponse{Kinds: kinds})
			}
		})))
	}

	routes.Handle("POST /v1/runs", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request modelwork.StartRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		started, err := controls.StartRun(r.Context(), owner, request)
		writeStartedRunOrError(w, started, err)
	})))

	for path, kind := range map[string]string{
		"/v1/jobs/{id}/facts/read": store.AgentPromptKindJobFacts,
		"/v1/jobs/{id}/brief/full": store.RunKindJobBriefFull,
		"/v1/jobs/{id}/cv":         store.AgentPromptKindJobCV,
	} {
		routes.Handle("POST "+path, requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := parsePathIDOrWriteNotFound(w, r)
			if !ok {
				return
			}
			started, err := controls.StartRun(r.Context(), owner, modelwork.StartRequest{Kind: kind, SubjectID: id})
			writeStartedRunOrError(w, started, err)
		})))
	}
}

// writeStartedRunOrError answers a start: the run once queued, a kind that
// is off or can't run yet with its reason, and a start that asks for what
// can't be with what's wrong.
func writeStartedRunOrError(w http.ResponseWriter, started modelwork.StartedRun, err error) {
	var cannotRun modelwork.CannotRunError
	var invalid modelwork.InvalidStartError
	switch {
	case err == nil:
		writeJSON(w, http.StatusAccepted, startedRunResponse{Queued: true, Run: started})
	case errors.Is(err, store.ErrJobNotFound), errors.Is(err, store.ErrCompanyNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
	case errors.Is(err, modelwork.ErrNotStartable), errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, modelwork.ErrNotSetUp):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	case errors.As(err, &cannotRun):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	}
}
