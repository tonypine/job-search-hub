package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TaskRun is one request the hub made to a model.
type TaskRun struct {
	ID               uuid.UUID       `json:"id"`
	Kind             string          `json:"kind"`
	SubjectID        *uuid.UUID      `json:"subject_id,omitempty"`
	SubjectType      *string         `json:"subject_type,omitempty"`
	BaseURL          string          `json:"base_url"`
	Provider         string          `json:"provider"`
	Model            string          `json:"model"`
	PromptVersion    *int            `json:"prompt_version,omitempty"`
	InputHash        string          `json:"input_hash"`
	Output           json.RawMessage `json:"output,omitempty"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	// CostUSD is what the provider said the run cost, as text so the
	// decimal is never rounded; nil when it said nothing.
	CostUSD        *string   `json:"cost_usd,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	DurationMS     int       `json:"duration_ms"`
	Outcome        string    `json:"outcome"`
	Error          string    `json:"error,omitempty"`
	StartedByOwner bool      `json:"started_by_owner"`
	// FallbackFor names the provider the run stood in for, when it ran on
	// its route's fallback.
	FallbackFor string `json:"fallback_for,omitempty"`
}

// NewTaskRun is a finished request to record. Its subject's type is its
// kind's, from the catalog of run kinds.
type NewTaskRun struct {
	Kind             string
	SubjectID        *uuid.UUID
	BaseURL          string
	Provider         string
	Model            string
	PromptID         *uuid.UUID
	PromptVersion    int
	InputHash        string
	Output           json.RawMessage
	PromptTokens     int
	CompletionTokens int
	// CostUSD is a decimal, or empty when the provider said nothing.
	CostUSD        string
	StartedAt      time.Time
	Duration       time.Duration
	Outcome        string
	Error          string
	StartedByOwner bool
	FallbackFor    string
}

// TaskRunFilter narrows the list: one kind, one outcome, and how many.
type TaskRunFilter struct {
	Kind    string
	Outcome string
	Limit   int
}

const (
	defaultTaskRunPageSize = 100
	maximumTaskRunPageSize = 1000
)

const taskRunColumns = `id, kind, subject_id, subject_type, base_url, provider, model, prompt_version, input_hash, output, prompt_tokens,
	completion_tokens, cost_usd::text, started_at, duration_ms, outcome, error, started_by_owner, fallback_for`

func scanTaskRun(row pgx.Row) (TaskRun, error) {
	var run TaskRun
	err := row.Scan(&run.ID, &run.Kind, &run.SubjectID, &run.SubjectType, &run.BaseURL, &run.Provider, &run.Model, &run.PromptVersion,
		&run.InputHash, &run.Output, &run.PromptTokens, &run.CompletionTokens, &run.CostUSD, &run.StartedAt, &run.DurationMS, &run.Outcome,
		&run.Error, &run.StartedByOwner, &run.FallbackFor)
	return run, err
}

// RecordTaskRun keeps a finished request. A run is a record of work done,
// not a change the owner made, so it leaves no change row.
func (s *Store) RecordTaskRun(ctx context.Context, input NewTaskRun) (TaskRun, error) {
	var promptVersion *int
	if input.PromptVersion > 0 {
		promptVersion = &input.PromptVersion
	}
	var output any
	if len(input.Output) > 0 {
		output = input.Output
	}
	var subjectType *string
	if input.SubjectID != nil {
		subjectType = getRunSubjectType(input.Kind)
	}
	return scanTaskRun(s.pool.QueryRow(ctx, `
		INSERT INTO task_runs (kind, subject_id, subject_type, base_url, provider, model, prompt_id, prompt_version, input_hash, output,
			prompt_tokens, completion_tokens, cost_usd, started_at, duration_ms, outcome, error, started_by_owner, fallback_for)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULLIF($13, '')::numeric, $14, $15, $16, $17, $18, $19)
		RETURNING `+taskRunColumns,
		input.Kind, input.SubjectID, subjectType, input.BaseURL, input.Provider, input.Model, input.PromptID, promptVersion, input.InputHash,
		output, input.PromptTokens, input.CompletionTokens, input.CostUSD, input.StartedAt, int(input.Duration.Milliseconds()), input.Outcome,
		input.Error, input.StartedByOwner, input.FallbackFor))
}

// ListTaskRuns returns the runs the filter keeps, the latest first.
func (s *Store) ListTaskRuns(ctx context.Context, filter TaskRunFilter) ([]TaskRun, error) {
	limit := filter.Limit
	if limit <= 0 || limit > maximumTaskRunPageSize {
		limit = defaultTaskRunPageSize
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskRunColumns+` FROM task_runs
		WHERE ($1 = '' OR kind = $1) AND ($2 = '' OR outcome = $2)
		ORDER BY started_at DESC LIMIT $3`, filter.Kind, filter.Outcome, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TaskRun, error) { return scanTaskRun(row) })
}
