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

// AgentRunKindProfileSeed builds the owner's knowledge base from the CV, the
// LinkedIn export and the answers library; it is also its prompt's kind.
const AgentRunKindProfileSeed = "profile_seed"

// AgentRunKindJobFix corrects a job's details from the owner's note; it is
// also its prompt's kind.
const AgentRunKindJobFix = "job_fix"

// companyAgentRunKinds are the agent runs for a company.
var companyAgentRunKinds = []string{AgentRunKindCompanyTriage, AgentRunKindJobFinder}

const (
	AgentRunRunning   = "running"
	AgentRunSucceeded = "succeeded"
	AgentRunFailed    = "failed"
)

type AgentRun struct {
	ID          uuid.UUID  `json:"id"`
	Kind        string     `json:"kind"`
	Input       string     `json:"input"`
	SubjectType *string    `json:"subject_type,omitempty"`
	SubjectID   *uuid.UUID `json:"subject_id,omitempty"`
	Status      string     `json:"status"`
	// Model is the Claude model the run was started with, from its kind's
	// route or the runner's choice; empty leaves it to Claude Code.
	Model              string    `json:"model,omitempty"`
	AgentPromptVersion *int      `json:"agent_prompt_version,omitempty"`
	TokenExpiresAt     time.Time `json:"token_expires_at"`
	ClaudeSessionID    string    `json:"claude_session_id,omitempty"`
	CostUSDEstimate    *string   `json:"cost_usd_estimate,omitempty"`
	// PromptTokens and CompletionTokens are what the session read and wrote,
	// as its result reported; zero until it finishes.
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	Result           json.RawMessage `json:"result,omitempty"`
	Error            string          `json:"error,omitempty"`
	StartedAt        time.Time       `json:"started_at"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
}

// The cost is read as text so the decimal Claude reported is never rounded
// through a float.
const agentRunColumns = `id, kind, input, subject_type, subject_id, status, model, agent_prompt_version, token_expires_at, claude_session_id,
	cost_usd_estimate::text, prompt_tokens, completion_tokens, result, error, started_at, finished_at`

func scanAgentRun(row pgx.Row) (AgentRun, error) {
	var run AgentRun
	err := row.Scan(&run.ID, &run.Kind, &run.Input, &run.SubjectType, &run.SubjectID, &run.Status, &run.Model, &run.AgentPromptVersion, &run.TokenExpiresAt,
		&run.ClaudeSessionID, &run.CostUSDEstimate, &run.PromptTokens, &run.CompletionTokens, &run.Result, &run.Error, &run.StartedAt, &run.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentRun{}, ErrAgentRunNotFound
	}
	return run, err
}

// StartAgentRun records a running agent run given version promptVersion of its
// prompt, on model, empty when Claude Code picks it. id is the id a run the
// owner started was given, nil for a new one. subjectID is the job or
// company the run is for, nil when it isn't known yet; its type is the
// kind's. A profile seed is for the owner's profile, which has no id. Only
// the token's hash is stored; the token itself exists only in the caller's
// hands.
func (s *Store) StartAgentRun(ctx context.Context, id *uuid.UUID, kind, input string, subjectID *uuid.UUID, model string, promptVersion int,
	tokenHash []byte, tokenExpiresAt time.Time) (AgentRun, error) {
	var subjectType *string
	if subjectID != nil || kind == AgentRunKindProfileSeed {
		subjectType = getRunSubjectType(kind)
	}
	return scanAgentRun(s.pool.QueryRow(ctx, `
		INSERT INTO agent_runs (id, kind, input, subject_type, subject_id, model, agent_prompt_version, token_hash, token_expires_at)
		VALUES (coalesce($1, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+agentRunColumns, id, kind, input, subjectType, subjectID, model, promptVersion, tokenHash, tokenExpiresAt))
}

type AgentRunOutcome struct {
	Status          string
	ClaudeSessionID string
	CostUSDEstimate string
	// PromptTokens counts what the session read, its cache included, and
	// CompletionTokens what it wrote.
	PromptTokens     int
	CompletionTokens int
	Result           json.RawMessage
	Error            string
}

// FinishAgentRun records how a running run ended. A finished run's token no
// longer verifies, since verification only accepts running runs. A company
// run started for a company the hub didn't know is linked to the first
// company it changed.
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
		WITH changed AS (
			SELECT entity_id FROM changes WHERE agent_run_id = $1 AND entity_type = 'company' ORDER BY id LIMIT 1
		)
		UPDATE agent_runs SET
			status = $2, claude_session_id = $3, cost_usd_estimate = $4::numeric, result = $5, error = $6, finished_at = now(),
			prompt_tokens = $9, completion_tokens = $10,
			subject_id = coalesce(agent_runs.subject_id, CASE WHEN kind = ANY($7) THEN (SELECT entity_id FROM changed) END),
			subject_type = CASE
				WHEN agent_runs.subject_id IS NULL AND kind = ANY($7) AND EXISTS (SELECT FROM changed) THEN $8
				ELSE agent_runs.subject_type END
		WHERE id = $1 AND status = 'running'
		RETURNING `+agentRunColumns,
		id, outcome.Status, outcome.ClaudeSessionID, cost, result, outcome.Error, companyAgentRunKinds, RunSubjectCompany,
		outcome.PromptTokens, outcome.CompletionTokens))
	if errors.Is(err, ErrAgentRunNotFound) {
		if _, getErr := s.GetAgentRun(ctx, id); getErr != nil {
			return AgentRun{}, getErr
		}
		return AgentRun{}, ErrAgentRunFinished
	}
	return run, err
}

// AgentRunAbandoned is the error of a run closed because it never reported
// an end before its token expired.
const AgentRunAbandoned = "abandoned"

// CloseAbandonedAgentRuns fails the runs still running after their token
// expired: their process died without reporting, and an expired token can't
// make any more calls. It returns the runs it closed.
func (s *Store) CloseAbandonedAgentRuns(ctx context.Context) ([]AgentRun, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE agent_runs SET status = 'failed', error = $1, finished_at = now()
		WHERE status = 'running' AND token_expires_at < now()
		RETURNING `+agentRunColumns, AgentRunAbandoned)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AgentRun, error) { return scanAgentRun(row) })
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

// ListRunningAgentRuns returns the runs still running whose token can still
// call the hub, the oldest first. A run whose token expired is abandoned.
func (s *Store) ListRunningAgentRuns(ctx context.Context) ([]AgentRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+agentRunColumns+` FROM agent_runs
		WHERE status = 'running' AND token_expires_at > now()
		ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AgentRun, error) { return scanAgentRun(row) })
}

// ListAgentRuns returns up to limit agent runs, the latest started first.
func (s *Store) ListAgentRuns(ctx context.Context, limit int) ([]AgentRun, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+agentRunColumns+` FROM agent_runs ORDER BY started_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AgentRun, error) { return scanAgentRun(row) })
}
