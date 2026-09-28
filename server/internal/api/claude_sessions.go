package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/prompts"
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

// claudeSessionContextResponse is what a session starts knowing, rendered
// from the prompt version it names.
type claudeSessionContextResponse struct {
	Context       string `json:"context"`
	PromptVersion int    `json:"prompt_version"`
}

// RegisterClaudeSessionRoutes adds the owner-only routes that keep the
// app's Claude sessions tied to the company or job each is about.
func RegisterClaudeSessionRoutes(routes *http.ServeMux, hub *store.Store, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
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

	handle("GET /v1/claude-sessions/{id}/context", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		session, err := hub.GetClaudeSession(r.Context(), id)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		var rendered prompts.Rendered
		if session.CompanyID != nil {
			rendered, err = prompts.RenderCompanySessionContext(r.Context(), hub, *session.CompanyID)
		} else {
			var details judgedJobDetails
			if details, err = getJudgedJobDetails(r.Context(), hub, rateSource, *session.JobID); err == nil {
				rendered, err = prompts.RenderJobSessionContext(r.Context(), hub, details, details.Job.CompanyID)
			}
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "render the context: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, claudeSessionContextResponse{Context: rendered.Body, PromptVersion: rendered.Version})
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
