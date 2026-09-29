package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/agents/companytriage"
	"github.com/tonypine/job-search-hub/server/agents/jobfinder"
	"github.com/tonypine/job-search-hub/server/agents/profileseed"
	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

const agentRunTokenLifetime = 30 * time.Minute

// agentRunResultSchemas are the kinds of run the owner can start, by the
// shape of the result each must answer with. A company run's input is the
// company: a name, a domain or a URL; a profile seed takes none.
var agentRunResultSchemas = map[string]string{
	store.AgentRunKindCompanyTriage: companytriage.ResultSchema,
	store.AgentRunKindJobFinder:     jobfinder.ResultSchema,
	store.AgentRunKindProfileSeed:   profileseed.ResultSchema,
}

// profileSeedInput stands for a profile seed's input, which is the owner's
// whole profile rather than a company.
const profileSeedInput = "owner profile"

type startAgentRunRequest struct {
	Kind  string `json:"kind"`
	Input string `json:"input"`
}

// startAgentRunResponse hands the runner everything a run needs: its token,
// the rendered prompt, and the schema its result must match.
type startAgentRunResponse struct {
	AgentRun     store.AgentRun  `json:"agent_run"`
	Token        string          `json:"token"`
	Prompt       string          `json:"prompt"`
	ResultSchema json.RawMessage `json:"result_schema"`
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
		resultSchema, knownKind := agentRunResultSchemas[request.Kind]
		isProfileSeed := request.Kind == store.AgentRunKindProfileSeed
		if !knownKind || (request.Input == "" && !isProfileSeed) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: `a run needs kind "company_triage" or "job_finder" with a company as input, or kind "profile_seed"`})
			return
		}

		var rendered prompts.Rendered
		var err error
		if isProfileSeed {
			request.Input = profileSeedInput
			rendered, err = prompts.RenderProfileSeedPrompt(r.Context(), hub)
		} else {
			rendered, err = prompts.RenderCompanyPrompt(r.Context(), hub, request.Kind, request.Input)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "render the prompt: " + err.Error()})
			return
		}
		token, tokenHash := tokens.NewAgentRunToken()
		run, err := hub.StartAgentRun(r.Context(), request.Kind, request.Input, rendered.Version, tokenHash, time.Now().Add(agentRunTokenLifetime))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, startAgentRunResponse{
			AgentRun: run, Token: token, Prompt: rendered.Body, ResultSchema: json.RawMessage(resultSchema),
		})
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
