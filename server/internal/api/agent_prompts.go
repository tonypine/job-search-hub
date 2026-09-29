package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// agentPromptSummary is one prompt kind with its active version, which is
// nil until the kind has one.
type agentPromptSummary struct {
	prompts.KindInfo
	Version   *int       `json:"version,omitempty"`
	Note      string     `json:"note,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type agentPromptsResponse struct {
	Prompts []agentPromptSummary `json:"prompts"`
}

type agentPromptVersionsResponse struct {
	Versions []store.AgentPrompt `json:"versions"`
}

type saveAgentPromptRequest struct {
	Body string `json:"body"`
	Note string `json:"note"`
}

// RegisterAgentPromptRoutes adds the owner-only routes for reading every
// prompt with its history and saving a new version. A new version keeps the
// previous one's result schema.
func RegisterAgentPromptRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/agent-prompts", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		summaries := make([]agentPromptSummary, 0, len(prompts.Kinds))
		for _, info := range prompts.Kinds {
			summary := agentPromptSummary{KindInfo: info}
			prompt, err := hub.GetLatestAgentPrompt(r.Context(), info.Kind)
			switch {
			case err == nil:
				summary.Version, summary.Note, summary.UpdatedAt = &prompt.Version, prompt.Note, &prompt.CreatedAt
			case !errors.Is(err, store.ErrAgentPromptNotFound):
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			summaries = append(summaries, summary)
		}
		writeJSON(w, http.StatusOK, agentPromptsResponse{Prompts: summaries})
	})))

	routes.Handle("GET /v1/agent-prompts/{kind}/versions", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		versions, err := hub.ListAgentPromptVersions(r.Context(), r.PathValue("kind"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, agentPromptVersionsResponse{Versions: versions})
	})))

	routes.Handle("POST /v1/agent-prompts/{kind}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request saveAgentPromptRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		prompt, err := hub.SaveAgentPrompt(r.Context(), store.Actor{Kind: store.ActorOwner}, store.NewAgentPrompt{
			Kind: r.PathValue("kind"), Body: request.Body, Note: request.Note,
		})
		switch {
		case errors.Is(err, store.ErrAgentPromptNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "no such prompt"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusCreated, prompt)
		}
	})))
}
