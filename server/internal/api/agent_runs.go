package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

const agentRunTokenLifetime = 30 * time.Minute

type startAgentRunRequest struct {
	Kind  string `json:"kind"`
	Input string `json:"input"`
}

type startAgentRunResponse struct {
	AgentRun store.AgentRun `json:"agent_run"`
	Token    string         `json:"token"`
}

type finishAgentRunRequest struct {
	Status          string          `json:"status"`
	ClaudeSessionID string          `json:"claude_session_id"`
	CostUSDEstimate json.Number     `json:"cost_usd_estimate"`
	Result          json.RawMessage `json:"result"`
	Error           string          `json:"error"`
}

type agentRunResponse struct {
	AgentRun store.AgentRun `json:"agent_run"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// RegisterAgentRunRoutes adds the agent-run routes, each wrapped in
// requireOwner so that only the owner can start, finish or read a run.
func RegisterAgentRunRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("POST /v1/agent-runs", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request startAgentRunRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		if request.Kind != store.AgentRunKindCompanyTriage || request.Input == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: `a run needs kind "company_triage" and a non-empty input`})
			return
		}

		token, tokenHash := tokens.NewAgentRunToken()
		run, err := hub.StartAgentRun(r.Context(), request.Kind, request.Input, tokenHash, time.Now().Add(agentRunTokenLifetime))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, startAgentRunResponse{AgentRun: run, Token: token})
	})))

	routes.Handle("POST /v1/agent-runs/{id}/finish", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: store.ErrAgentRunNotFound.Error()})
			return
		}
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		var request finishAgentRunRequest
		if err := decoder.Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}

		run, err := hub.FinishAgentRun(r.Context(), id, store.AgentRunOutcome{
			Status:          request.Status,
			ClaudeSessionID: request.ClaudeSessionID,
			CostUSDEstimate: request.CostUSDEstimate.String(),
			Result:          request.Result,
			Error:           request.Error,
		})
		switch {
		case errors.Is(err, store.ErrAgentRunNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		case errors.Is(err, store.ErrAgentRunFinished):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, agentRunResponse{AgentRun: run})
		}
	})))

	routes.Handle("GET /v1/agent-runs/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: store.ErrAgentRunNotFound.Error()})
			return
		}
		run, err := hub.GetAgentRun(r.Context(), id)
		switch {
		case errors.Is(err, store.ErrAgentRunNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, agentRunResponse{AgentRun: run})
		}
	})))
}
