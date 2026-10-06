package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type pipelineResponse struct {
	Phases []store.PipelinePhase `json:"phases"`
	Cards  []store.PipelineCard  `json:"cards"`
}

type addApplicationRequest struct {
	JobID     *uuid.UUID `json:"job_id"`
	CompanyID *uuid.UUID `json:"company_id"`
	Notes     string     `json:"notes"`
}

type applicationResponse struct {
	Application store.Application `json:"application"`
	Created     bool              `json:"created,omitempty"`
}

// updateApplicationRequest moves the application when phase_id is set, and
// replaces its notes when notes is set.
type updateApplicationRequest struct {
	PhaseID      *uuid.UUID `json:"phase_id"`
	ClosedReason string     `json:"closed_reason"`
	Notes        *string    `json:"notes"`
}

type addPhaseRequest struct {
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
}

type renamePhaseRequest struct {
	Name string `json:"name"`
}

// dismissApplicationRequest carries the owner's note on why the card isn't a
// good fit; it may be empty.
type dismissApplicationRequest struct {
	Note string `json:"note"`
}

type followUpRequest struct {
	Note string `json:"note"`
}

// outreachRequest records a cold message to someone at a company; sent_on is
// the day it went out, as YYYY-MM-DD, now when absent.
type outreachRequest struct {
	Note   string `json:"note"`
	SentOn string `json:"sent_on"`
}

// setFollowUpDaysRequest sets a phase's follow-up interval; a null days
// stops the phase asking for follow-ups.
type setFollowUpDaysRequest struct {
	Days *int `json:"days"`
}

type reorderPhasesRequest struct {
	PhaseIDs []uuid.UUID `json:"phase_ids"`
}

type phasesResponse struct {
	Phases []store.PipelinePhase `json:"phases"`
}

// RegisterPipelineRoutes adds the owner-only routes for the pipeline board.
func RegisterPipelineRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	handle := func(pattern string, handler http.HandlerFunc) { routes.Handle(pattern, requireOwner(handler)) }

	handle("GET /v1/pipeline", func(w http.ResponseWriter, r *http.Request) {
		phases, err := hub.ListPipelinePhases(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		listCards := hub.ListPipelineCards
		if r.URL.Query().Get("dismissed") == "true" {
			listCards = hub.ListDismissedPipelineCards
		}
		cards, err := listCards(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, pipelineResponse{Phases: phases, Cards: cards})
	})

	handle("POST /v1/applications", func(w http.ResponseWriter, r *http.Request) {
		var request addApplicationRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		application, created, err := hub.AddApplication(r.Context(), owner, store.ApplicationInput{JobID: request.JobID, CompanyID: request.CompanyID, Notes: request.Notes})
		if err != nil {
			writeStoreError(w, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, applicationResponse{Application: application, Created: created})
	})

	handle("PATCH /v1/applications/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		var request updateApplicationRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		if request.PhaseID == nil && request.Notes == nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "give phase_id to move the application, or notes to replace them"})
			return
		}
		var application store.Application
		var err error
		if request.PhaseID != nil {
			application, err = hub.MoveApplication(r.Context(), owner, id, *request.PhaseID, request.ClosedReason)
		}
		if err == nil && request.Notes != nil {
			application, err = hub.UpdateApplicationNotes(r.Context(), owner, id, *request.Notes)
		}
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, applicationResponse{Application: application})
	})

	handle("POST /v1/applications/{id}/dismiss", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		var request dismissApplicationRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		application, err := hub.DismissApplication(r.Context(), owner, id, request.Note)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, applicationResponse{Application: application})
	})

	handle("POST /v1/applications/{id}/restore", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		application, err := hub.RestoreApplication(r.Context(), owner, id)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, applicationResponse{Application: application})
	})

	handle("POST /v1/applications/{id}/follow-ups", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		var request followUpRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		application, err := hub.RecordFollowUp(r.Context(), owner, id, request.Note)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, applicationResponse{Application: application})
	})

	handle("POST /v1/companies/{id}/outreach", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		var request outreachRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		sentAt := time.Now()
		if request.SentOn != "" {
			var err error
			if sentAt, err = time.ParseInLocation(time.DateOnly, request.SentOn, time.Local); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sent_on must be a date as YYYY-MM-DD"})
				return
			}
		}
		application, created, err := hub.RecordOutreach(r.Context(), owner, id, request.Note, sentAt, "")
		if err != nil {
			writeStoreError(w, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, applicationResponse{Application: application, Created: created})
	})

	handle("PUT /v1/pipeline/phases/{id}/follow-up", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		var request setFollowUpDaysRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		phase, err := hub.SetPipelinePhaseFollowUpDays(r.Context(), owner, id, request.Days)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, phase)
	})

	handle("POST /v1/pipeline/phases", func(w http.ResponseWriter, r *http.Request) {
		var request addPhaseRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		phase, err := hub.AddPipelinePhase(r.Context(), owner, request.Name, request.IsClosed)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, phase)
	})

	handle("PATCH /v1/pipeline/phases/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		var request renamePhaseRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		phase, err := hub.RenamePipelinePhase(r.Context(), owner, id, request.Name)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, phase)
	})

	handle("PUT /v1/pipeline/phases/order", func(w http.ResponseWriter, r *http.Request) {
		var request reorderPhasesRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		phases, err := hub.ReorderPipelinePhases(r.Context(), owner, request.PhaseIDs)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, phasesResponse{Phases: phases})
	})

	handle("DELETE /v1/pipeline/phases/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		if err := hub.DeletePipelinePhase(r.Context(), owner, id); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func decodeBodyOrWriteBadRequest(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
		return false
	}
	return true
}

func parsePathIDOrWriteNotFound(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return uuid.UUID{}, false
	}
	return id, true
}

// writeStoreError answers with the status each store error stands for.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrApplicationNotFound), errors.Is(err, store.ErrPipelinePhaseNotFound),
		errors.Is(err, store.ErrJobNotFound), errors.Is(err, store.ErrCompanyNotFound), errors.Is(err, store.ErrClaudeSessionNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, store.ErrPipelinePhaseInUse), errors.Is(err, store.ErrPipelinePhaseNameUsed):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	}
}
