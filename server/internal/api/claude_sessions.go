package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const defaultClaudeSessionListSize = 50

type createClaudeSessionRequest struct {
	CompanyID *uuid.UUID `json:"company_id"`
	JobID     *uuid.UUID `json:"job_id"`
}

type claudeSessionsResponse struct {
	Sessions []store.ClaudeSession `json:"sessions"`
}

// RegisterClaudeSessionRoutes adds the owner-only routes that keep the
// app's Claude sessions tied to the company or job each is about.
func RegisterClaudeSessionRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	handle := func(pattern string, handler http.HandlerFunc) { routes.Handle(pattern, requireOwner(handler)) }

	handle("POST /v1/claude-sessions", func(w http.ResponseWriter, r *http.Request) {
		var request createClaudeSessionRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		session, err := hub.CreateClaudeSession(r.Context(), owner, store.ClaudeSessionSubject{CompanyID: request.CompanyID, JobID: request.JobID})
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, session)
	})

	handle("GET /v1/claude-sessions", func(w http.ResponseWriter, r *http.Request) {
		var subject store.ClaudeSessionSubject
		for name, into := range map[string]**uuid.UUID{"company_id": &subject.CompanyID, "job_id": &subject.JobID} {
			if raw := r.URL.Query().Get(name); raw != "" {
				id, err := uuid.Parse(raw)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, errorResponse{Error: name + " must be a uuid"})
					return
				}
				*into = &id
			}
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > defaultClaudeSessionListSize {
			limit = defaultClaudeSessionListSize
		}
		sessions, err := hub.ListClaudeSessions(r.Context(), subject, limit)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, claudeSessionsResponse{Sessions: sessions})
	})

	handle("GET /v1/claude-sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		session, err := hub.GetClaudeSession(r.Context(), id)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, session)
	})

	for _, event := range []struct {
		path   string
		record func(*http.Request, uuid.UUID) (store.ClaudeSession, error)
	}{
		{"start", func(r *http.Request, id uuid.UUID) (store.ClaudeSession, error) {
			return hub.RecordClaudeSessionStart(r.Context(), id)
		}},
		{"stop", func(r *http.Request, id uuid.UUID) (store.ClaudeSession, error) {
			return hub.RecordClaudeSessionStop(r.Context(), id)
		}},
	} {
		handle("POST /v1/claude-sessions/{id}/"+event.path, func(w http.ResponseWriter, r *http.Request) {
			id, ok := parsePathIDOrWriteNotFound(w, r)
			if !ok {
				return
			}
			session, err := event.record(r, id)
			if err != nil {
				writeStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, session)
		})
	}
}
