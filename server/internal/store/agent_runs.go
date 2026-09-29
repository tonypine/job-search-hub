package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrAgentRunNotFound = errors.New("agent run not found")
	ErrAgentRunFinished = errors.New("agent run already finished")
)

const AgentRunKindCompanyTriage = "company_triage"

// AgentRunKindJobFinder finds a company's open roles; it is also its
// prompt's kind.
const AgentRunKindJobFinder = "job_finder"

const (
	AgentRunRunning   = "running"
	AgentRunSucceeded = "succeeded"
	AgentRunFailed    = "failed"
)

type AgentRun struct {
	ID                 uuid.UUID       `json:"id"`
	Kind               string          `json:"kind"`
	Input              string          `json:"input"`
	Status             string          `json:"status"`
	AgentPromptVersion *int            `json:"agent_prompt_version,omitempty"`
	TokenExpiresAt     time.Time       `json:"token_expires_at"`
	ClaudeSessionID    string          `json:"claude_session_id,omitempty"`
	CostUSDEstimate    *string         `json:"cost_usd_estimate,omitempty"`
	Result             json.RawMessage `json:"result,omitempty"`
	Error              string          `json:"error,omitempty"`
	StartedAt          time.Time       `json:"started_at"`
	FinishedAt         *time.Time      `json:"finished_at,omitempty"`
}

// The cost is read as text so the decimal Claude reported is never rounded
// through a float.
const agentRunColumns = `id, kind, input, status, agent_prompt_version, token_expires_at, claude_session_id, cost_usd_estimate::text, result, error, started_at, finished_at`

func scanAgentRun(row pgx.Row) (AgentRun, error) {
	var run AgentRun
	err := row.Scan(&run.ID, &run.Kind, &run.Input, &run.Status, &run.AgentPromptVersion, &run.TokenExpiresAt, &run.ClaudeSessionID,
		&run.CostUSDEstimate, &run.Result, &run.Error, &run.StartedAt, &run.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentRun{}, ErrAgentRunNotFound
	}
	return run, err
}

// StartAgentRun records a running agent run given version promptVersion of its
// prompt. Only the token's hash is stored; the token itself exists only in the
// caller's hands.
func (s *Store) StartAgentRun(ctx context.Context, kind, input string, promptVersion int, tokenHash []byte, tokenExpiresAt time.Time) (AgentRun, error) {
	return scanAgentRun(s.pool.QueryRow(ctx, `
		INSERT INTO agent_runs (kind, input, agent_prompt_version, token_hash, token_expires_at) VALUES ($1, $2, $3, $4, $5)
		RETURNING `+agentRunColumns, kind, input, promptVersion, tokenHash, tokenExpiresAt))
}

type AgentRunOutcome struct {
	Status          string
	ClaudeSessionID string
	CostUSDEstimate string
	Result          json.RawMessage
	Error           string
}

// FinishAgentRun records how a running run ended. A finished run's token no
// longer verifies, since verification only accepts running runs.
func (s *Store) FinishAgentRun(ctx context.Context, id uuid.UUID, outcome AgentRunOutcome) (AgentRun, error) {
	if outcome.Status != AgentRunSucceeded && outcome.Status != AgentRunFailed {
		return AgentRun{}, fmt.Errorf("a run finishes as %q or %q, not %q", AgentRunSucceeded, AgentRunFailed, outcome.Status)
	}
	var cost *string
	if outcome.CostUSDEstimate != "" {
		cost = &outcome.CostUSDEstimate
	}
	var result []byte
	if len(outcome.Result) > 0 {
		result = outcome.Result
	}

	run, err := scanAgentRun(s.pool.QueryRow(ctx, `
		UPDATE agent_runs SET
			status = $2, claude_session_id = $3, cost_usd_estimate = $4::numeric, result = $5, error = $6, finished_at = now()
		WHERE id = $1 AND status = 'running'
		RETURNING `+agentRunColumns,
		id, outcome.Status, outcome.ClaudeSessionID, cost, result, outcome.Error))
	if errors.Is(err, ErrAgentRunNotFound) {
		if _, getErr := s.GetAgentRun(ctx, id); getErr != nil {
			return AgentRun{}, getErr
		}
		return AgentRun{}, ErrAgentRunFinished
	}
	return run, err
}

func (s *Store) GetAgentRun(ctx context.Context, id uuid.UUID) (AgentRun, error) {
	return scanAgentRun(s.pool.QueryRow(ctx, `SELECT `+agentRunColumns+` FROM agent_runs WHERE id = $1`, id))
}

// GetRunningAgentRunByTokenHash finds the run a token belongs to, as long as
// the run is still running and its token has not expired.
func (s *Store) GetRunningAgentRunByTokenHash(ctx context.Context, tokenHash []byte) (AgentRun, error) {
	return scanAgentRun(s.pool.QueryRow(ctx, `
		SELECT `+agentRunColumns+` FROM agent_runs
		WHERE token_hash = $1 AND status = 'running' AND token_expires_at > now()`, tokenHash))
}
