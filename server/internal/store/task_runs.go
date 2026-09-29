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
	BaseURL          string          `json:"base_url"`
	Model            string          `json:"model"`
	PromptVersion    *int            `json:"prompt_version,omitempty"`
	InputHash        string          `json:"input_hash"`
	Output           json.RawMessage `json:"output,omitempty"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	StartedAt        time.Time       `json:"started_at"`
	DurationMS       int             `json:"duration_ms"`
	Outcome          string          `json:"outcome"`
	Error            string          `json:"error,omitempty"`
}

// NewTaskRun is a finished request to record.
type NewTaskRun struct {
	Kind             string
	SubjectID        *uuid.UUID
	BaseURL          string
	Model            string
	PromptID         *uuid.UUID
	PromptVersion    int
	InputHash        string
	Output           json.RawMessage
	PromptTokens     int
	CompletionTokens int
	StartedAt        time.Time
	Duration         time.Duration
	Outcome          string
	Error            string
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

const taskRunColumns = `id, kind, subject_id, base_url, model, prompt_version, input_hash, output, prompt_tokens, completion_tokens,
	started_at, duration_ms, outcome, error`

func scanTaskRun(row pgx.Row) (TaskRun, error) {
	var run TaskRun
	err := row.Scan(&run.ID, &run.Kind, &run.SubjectID, &run.BaseURL, &run.Model, &run.PromptVersion, &run.InputHash, &run.Output,
		&run.PromptTokens, &run.CompletionTokens, &run.StartedAt, &run.DurationMS, &run.Outcome, &run.Error)
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
	return scanTaskRun(s.pool.QueryRow(ctx, `
		INSERT INTO task_runs (kind, subject_id, base_url, model, prompt_id, prompt_version, input_hash, output, prompt_tokens,
			completion_tokens, started_at, duration_ms, outcome, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING `+taskRunColumns,
		input.Kind, input.SubjectID, input.BaseURL, input.Model, input.PromptID, promptVersion, input.InputHash, output,
		input.PromptTokens, input.CompletionTokens, input.StartedAt, int(input.Duration.Milliseconds()), input.Outcome, input.Error))
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
