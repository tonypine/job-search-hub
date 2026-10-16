package store

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The records a run comes from.
const (
	RunRecordModelCall = "model_call"
	RunRecordAgentRun  = "agent_run"
)

// The outcomes of a run, from either record: a model call succeeds, fails or
// answers invalidly; an agent run is running until it succeeds or fails.
const (
	RunOutcomeSucceeded = "succeeded"
	RunOutcomeFailed    = "failed"
	RunOutcomeInvalid   = "invalid"
	RunOutcomeRunning   = "running"
)

// RunSubject is what a run was for, as the owner would name it: a job with
// its company, a company, a message or a conversation, or the profile, which
// has no id. Its title is empty once the subject is gone.
type RunSubject struct {
	Type        string     `json:"type"`
	ID          *uuid.UUID `json:"id,omitempty"`
	Title       string     `json:"title"`
	CompanyID   *uuid.UUID `json:"company_id,omitempty"`
	CompanyName string     `json:"company_name,omitempty"`
}

// Run is one run from either record: a model call or a Claude agent run.
type Run struct {
	ID      uuid.UUID   `json:"id"`
	Record  string      `json:"record"`
	Kind    string      `json:"kind"`
	Subject *RunSubject `json:"subject,omitempty"`
	// Model is empty for an agent run, whose model Claude Code picks.
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	StartedAt time.Time `json:"started_at"`
	// DurationMS is nil while an agent run runs.
	DurationMS       *int   `json:"duration_ms,omitempty"`
	Outcome          string `json:"outcome"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	// CostUSDEstimate is what Claude reported an agent run cost, as text so
	// the decimal is never rounded.
	CostUSDEstimate *string `json:"cost_usd_estimate,omitempty"`
	Error           string  `json:"error,omitempty"`
	StartedByOwner  bool    `json:"started_by_owner"`
}

// RunFilter narrows the runs: to a kind, to a subject by id (and type, when
// set), to an outcome, and to those started from Since until before Until.
// Cursor continues a list where its last page ended.
type RunFilter struct {
	Kind        string
	SubjectType string
	SubjectID   *uuid.UUID
	Outcome     string
	Since       *time.Time
	Until       *time.Time
	Cursor      string
	Limit       int
}

// RunPage is a page of runs, the latest first, and the cursor of the next
// page, empty after the last.
type RunPage struct {
	Runs       []Run  `json:"runs"`
	NextCursor string `json:"next_cursor"`
}

const (
	defaultRunPageSize = 50
	maximumRunPageSize = 200
)

// ErrInvalidRunCursor is a cursor no page of runs gave.
var ErrInvalidRunCursor = errors.New("the cursor isn't one a page of runs gave")

// The cursor is the last run's start and id: runs sort by both, the latest
// first, so the next page is the runs before it.
func encodeRunCursor(run Run) string {
	return base64.RawURLEncoding.EncodeToString([]byte(run.StartedAt.UTC().Format(time.RFC3339Nano) + "|" + run.ID.String()))
}

func decodeRunCursor(cursor string) (time.Time, uuid.UUID, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.UUID{}, ErrInvalidRunCursor
	}
	startedText, idText, found := strings.Cut(string(decoded), "|")
	startedAt, timeErr := time.Parse(time.RFC3339Nano, startedText)
	id, idErr := uuid.Parse(idText)
	if !found || timeErr != nil || idErr != nil {
		return time.Time{}, uuid.UUID{}, ErrInvalidRunCursor
	}
	return startedAt, id, nil
}

// runsQuery merges both records into one shape. An agent run is always one
// the owner started: the hub starts none by itself.
const runsQuery = `
	WITH runs AS (
		SELECT id, 'model_call' AS record, kind, subject_type, subject_id, model, provider, started_at, duration_ms, outcome,
			prompt_tokens, completion_tokens, NULL::text AS cost_usd_estimate, error, started_by_owner
		FROM task_runs
		UNION ALL
		SELECT id, 'agent_run', kind, subject_type, subject_id, '', 'Claude', started_at,
			CASE WHEN finished_at IS NOT NULL THEN round(extract(epoch FROM finished_at - started_at) * 1000)::integer END, status,
			0, 0, cost_usd_estimate::text, error, true
		FROM agent_runs
	)
	SELECT runs.id, runs.record, runs.kind, runs.subject_type, runs.subject_id,
		coalesce(CASE runs.subject_type
			WHEN 'job' THEN jobs.title
			WHEN 'company' THEN companies.name
			WHEN 'mail_message' THEN mail_messages.subject
			WHEN 'linkedin_conversation' THEN linkedin_conversations.title
			WHEN 'profile' THEN 'Your profile'
		END, ''),
		companies.id, coalesce(companies.name, ''),
		runs.model, runs.provider, runs.started_at, runs.duration_ms, runs.outcome, runs.prompt_tokens, runs.completion_tokens,
		runs.cost_usd_estimate, runs.error, runs.started_by_owner
	FROM runs
	LEFT JOIN jobs ON runs.subject_type = 'job' AND jobs.id = runs.subject_id
	LEFT JOIN companies ON companies.id = CASE WHEN runs.subject_type = 'company' THEN runs.subject_id ELSE jobs.company_id END
	LEFT JOIN mail_messages ON runs.subject_type = 'mail_message' AND mail_messages.id = runs.subject_id
	LEFT JOIN linkedin_conversations ON runs.subject_type = 'linkedin_conversation' AND linkedin_conversations.id = runs.subject_id
	WHERE ($1 = '' OR runs.kind = $1)
		AND ($2 = '' OR runs.subject_type = $2)
		AND ($3::uuid IS NULL OR runs.subject_id = $3)
		AND ($4 = '' OR runs.outcome = $4)
		AND ($5::timestamptz IS NULL OR runs.started_at >= $5)
		AND ($6::timestamptz IS NULL OR runs.started_at < $6)
		AND ($7::timestamptz IS NULL OR (runs.started_at, runs.id) < ($7, $8::uuid))
	ORDER BY runs.started_at DESC, runs.id DESC
	LIMIT $9`

func scanRun(row pgx.CollectableRow) (Run, error) {
	var run Run
	var subjectType *string
	var subjectID, companyID *uuid.UUID
	var subjectTitle, companyName string
	err := row.Scan(&run.ID, &run.Record, &run.Kind, &subjectType, &subjectID, &subjectTitle, &companyID, &companyName,
		&run.Model, &run.Provider, &run.StartedAt, &run.DurationMS, &run.Outcome, &run.PromptTokens, &run.CompletionTokens,
		&run.CostUSDEstimate, &run.Error, &run.StartedByOwner)
	if err != nil || subjectType == nil {
		return run, err
	}
	run.Subject = &RunSubject{Type: *subjectType, ID: subjectID, Title: subjectTitle}
	if *subjectType == RunSubjectJob && companyID != nil {
		run.Subject.CompanyID, run.Subject.CompanyName = companyID, companyName
	}
	return run, nil
}

// ListRuns returns a page of the runs the filter keeps, from both records,
// the latest first. A run with no subject is one whose kind has none, or
// whose subject couldn't be told.
func (s *Store) ListRuns(ctx context.Context, filter RunFilter) (RunPage, error) {
	limit := filter.Limit
	if limit <= 0 || limit > maximumRunPageSize {
		limit = defaultRunPageSize
	}
	var cursorStartedAt *time.Time
	var cursorID *uuid.UUID
	if filter.Cursor != "" {
		startedAt, id, err := decodeRunCursor(filter.Cursor)
		if err != nil {
			return RunPage{}, err
		}
		cursorStartedAt, cursorID = &startedAt, &id
	}
	rows, err := s.pool.Query(ctx, runsQuery, filter.Kind, filter.SubjectType, filter.SubjectID, filter.Outcome, filter.Since, filter.Until,
		cursorStartedAt, cursorID, limit+1)
	if err != nil {
		return RunPage{}, err
	}
	runs, err := pgx.CollectRows(rows, scanRun)
	if err != nil {
		return RunPage{}, err
	}
	page := RunPage{Runs: runs}
	if len(runs) > limit {
		page.Runs = runs[:limit]
		page.NextCursor = encodeRunCursor(page.Runs[limit-1])
	}
	return page, nil
}
