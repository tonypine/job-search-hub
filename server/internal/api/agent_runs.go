package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/agents/companytriage"
	"github.com/tonypine/job-search-hub/server/agents/jobfinder"
	"github.com/tonypine/job-search-hub/server/agents/jobfixer"
	"github.com/tonypine/job-search-hub/server/agents/profileseed"
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

const (
	agentRunTokenLifetime   = 30 * time.Minute
	defaultAgentRunPageSize = 100
	maximumAgentRunPageSize = 500
)

type agentRunsResponse struct {
	Runs []store.AgentRun `json:"runs"`
}

// agentRunResultSchemas are the kinds of run the owner can start, by the
// shape of the result each must answer with. A company run's input is the
// company: a name, a domain or a URL; a profile seed takes none.
var agentRunResultSchemas = map[string]string{
	store.AgentRunKindCompanyTriage: companytriage.ResultSchema,
	store.AgentRunKindJobFinder:     jobfinder.ResultSchema,
	store.AgentRunKindProfileSeed:   profileseed.ResultSchema,
	store.AgentRunKindJobFix:        jobfixer.ResultSchema,
}

// profileSeedInput stands for a profile seed's input, which is the owner's
// whole profile rather than a company.
const profileSeedInput = "owner profile"

type startAgentRunRequest struct {
	Kind  string `json:"kind"`
	Input string `json:"input"`
	// Note is the owner's word on what to fix, for a job_fix run.
	Note string `json:"note"`
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
// requireOwner so that only the owner can start, finish or read a run. No run
// starts while drainer drains; a nil drainer never does.
func RegisterAgentRunRoutes(routes *http.ServeMux, hub *store.Store, drainer *drain.Drain, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("POST /v1/agent-runs", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !writeDrainingOrContinue(w, drainer) {
			return
		}
		var request startAgentRunRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		resultSchema, knownKind := agentRunResultSchemas[request.Kind]
		isProfileSeed := request.Kind == store.AgentRunKindProfileSeed
		if !knownKind || (request.Input == "" && !isProfileSeed) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: `a run needs kind "company_triage" or "job_finder" with a company as input, ` +
				`kind "job_fix" with a job id as input and a note, or kind "profile_seed"`})
			return
		}

		var rendered prompts.Rendered
		var err error
		switch {
		case isProfileSeed:
			request.Input = profileSeedInput
			rendered, err = prompts.RenderProfileSeedPrompt(r.Context(), hub)
		case request.Kind == store.AgentRunKindJobFix:
			jobID, parseErr := uuid.Parse(request.Input)
			if parseErr != nil || strings.TrimSpace(request.Note) == "" {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "a job fix needs the job's id as input and a note"})
				return
			}
			rendered, err = prompts.RenderJobFixPrompt(r.Context(), hub, jobID, request.Note)
			if errors.Is(err, store.ErrJobNotFound) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "no such job"})
				return
			}
		default:
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

	routes.Handle("GET /v1/agent-runs", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > maximumAgentRunPageSize {
			limit = defaultAgentRunPageSize
		}
		runs, err := hub.ListAgentRuns(r.Context(), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, agentRunsResponse{Runs: runs})
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
