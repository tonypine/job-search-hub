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

var ErrComparisonNotFound = errors.New("comparison not found")

const (
	ComparisonSourceImported = "imported"
	ComparisonSourceRoute    = "route"
	ComparisonSourceClaude   = "claude"

	ComparisonStatusRunning = "running"
	ComparisonStatusDone    = "done"
)

// Comparison runs one task kind on the same jobs through its stacks, in
// order; the first stack is what the others are measured against.
type Comparison struct {
	ID        uuid.UUID         `json:"id"`
	TaskKind  string            `json:"task_kind"`
	Title     string            `json:"title"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	Stacks    []ComparisonStack `json:"stacks"`
	JobIDs    []uuid.UUID       `json:"job_ids"`
}

// ComparisonStack is one way of answering: a model on a provider, Claude
// through the CLI, or answers imported from elsewhere.
type ComparisonStack struct {
	ID         uuid.UUID  `json:"id"`
	Label      string     `json:"label"`
	Source     string     `json:"source"`
	ProviderID *uuid.UUID `json:"provider_id,omitempty"`
	Model      string     `json:"model,omitempty"`
}

// ComparisonAnswer is one stack's answer on one job, or why it has none.
type ComparisonAnswer struct {
	StackID uuid.UUID       `json:"stack_id"`
	JobID   uuid.UUID       `json:"job_id"`
	Answer  json.RawMessage `json:"answer,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// ComparisonVerdict is the owner's judgment of one stack's field on one job.
type ComparisonVerdict struct {
	StackID uuid.UUID `json:"stack_id"`
	JobID   uuid.UUID `json:"job_id"`
	Field   string    `json:"field"`
	Verdict string    `json:"verdict"`
}

// NewComparison is a comparison to create: its stacks in order, and its jobs.
type NewComparison struct {
	TaskKind string
	Title    string
	Stacks   []ComparisonStack
	JobIDs   []uuid.UUID
}

// CreateComparison stores the comparison with its stacks and jobs, running,
// and returns it with the stacks' ids.
func (s *Store) CreateComparison(ctx context.Context, actor Actor, input NewComparison) (Comparison, error) {
	if len(input.Stacks) < 2 || len(input.JobIDs) == 0 {
		return Comparison{}, errors.New("a comparison needs two stacks or more, and a job")
	}
	comparison := Comparison{TaskKind: input.TaskKind, Title: input.Title, Status: ComparisonStatusRunning, JobIDs: input.JobIDs}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO comparisons (task_kind, title) VALUES ($1, $2) RETURNING id, created_at`,
			input.TaskKind, input.Title).Scan(&comparison.ID, &comparison.CreatedAt); err != nil {
			return err
		}
		for position, stack := range input.Stacks {
			if err := tx.QueryRow(ctx, `
				INSERT INTO comparison_stacks (comparison_id, position, label, source, provider_id, model) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
				comparison.ID, position, stack.Label, stack.Source, stack.ProviderID, stack.Model).Scan(&stack.ID); err != nil {
				return err
			}
			comparison.Stacks = append(comparison.Stacks, stack)
		}
		for position, jobID := range input.JobIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO comparison_jobs (comparison_id, job_id, position) VALUES ($1, $2, $3)`, comparison.ID, jobID, position); err != nil {
				if isForeignKeyViolation(err) {
					return fmt.Errorf("%w: %s", ErrJobNotFound, jobID)
				}
				return err
			}
		}
		return insertChange(ctx, tx, actor, change{entityType: "comparison", entityID: comparison.ID, operation: "create", after: map[string]string{"title": input.Title}})
	})
	return comparison, err
}

// SaveComparisonAnswer keeps a stack's answer on a job, or the error that
// left it without one.
func (s *Store) SaveComparisonAnswer(ctx context.Context, answer ComparisonAnswer) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO comparison_answers (stack_id, job_id, answer, error) VALUES ($1, $2, $3, $4)
		ON CONFLICT (stack_id, job_id) DO UPDATE SET answer = EXCLUDED.answer, error = EXCLUDED.error`,
		answer.StackID, answer.JobID, convertEmptyJSONToNull(answer.Answer), answer.Error)
	return err
}

// FinishComparison marks the comparison done.
func (s *Store) FinishComparison(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE comparisons SET status = 'done' WHERE id = $1`, id)
	return err
}

// SaveComparisonVerdict records the owner's verdict on a stack's field for a
// job, replacing an earlier one.
func (s *Store) SaveComparisonVerdict(ctx context.Context, comparisonID uuid.UUID, verdict ComparisonVerdict) error {
	if verdict.Verdict != "right" && verdict.Verdict != "wrong" {
		return errors.New("a verdict is right or wrong")
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO comparison_verdicts (stack_id, job_id, field, verdict)
		SELECT $2, $3, $4, $5 WHERE EXISTS (SELECT 1 FROM comparison_stacks WHERE id = $2 AND comparison_id = $1)
		ON CONFLICT (stack_id, job_id, field) DO UPDATE SET verdict = EXCLUDED.verdict, judged_at = now()`,
		comparisonID, verdict.StackID, verdict.JobID, verdict.Field, verdict.Verdict)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrComparisonNotFound
	}
	return nil
}

// ComparisonRecord is a comparison with every answer and verdict.
type ComparisonRecord struct {
	Comparison
	Answers  []ComparisonAnswer  `json:"answers"`
	Verdicts []ComparisonVerdict `json:"verdicts"`
}

// GetComparison returns the comparison with its stacks, jobs, answers and verdicts.
func (s *Store) GetComparison(ctx context.Context, id uuid.UUID) (ComparisonRecord, error) {
	var record ComparisonRecord
	err := s.pool.QueryRow(ctx, `SELECT id, task_kind, title, status, created_at FROM comparisons WHERE id = $1`, id).
		Scan(&record.ID, &record.TaskKind, &record.Title, &record.Status, &record.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ComparisonRecord{}, ErrComparisonNotFound
	}
	if err != nil {
		return ComparisonRecord{}, err
	}
	if record.Stacks, err = s.listComparisonStacks(ctx, id); err != nil {
		return ComparisonRecord{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT job_id FROM comparison_jobs WHERE comparison_id = $1 ORDER BY position`, id)
	if err != nil {
		return ComparisonRecord{}, err
	}
	if record.JobIDs, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID]); err != nil {
		return ComparisonRecord{}, err
	}
	rows, err = s.pool.Query(ctx, `
		SELECT answers.stack_id, answers.job_id, answers.answer, answers.error
		FROM comparison_answers answers JOIN comparison_stacks stacks ON stacks.id = answers.stack_id WHERE stacks.comparison_id = $1`, id)
	if err != nil {
		return ComparisonRecord{}, err
	}
	if record.Answers, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ComparisonAnswer]); err != nil {
		return ComparisonRecord{}, err
	}
	rows, err = s.pool.Query(ctx, `
		SELECT verdicts.stack_id, verdicts.job_id, verdicts.field, verdicts.verdict
		FROM comparison_verdicts verdicts JOIN comparison_stacks stacks ON stacks.id = verdicts.stack_id WHERE stacks.comparison_id = $1`, id)
	if err != nil {
		return ComparisonRecord{}, err
	}
	record.Verdicts, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ComparisonVerdict])
	return record, err
}

// ListComparisons returns every comparison with its stacks and jobs, the
// newest first.
func (s *Store) ListComparisons(ctx context.Context) ([]Comparison, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM comparisons ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	comparisons := make([]Comparison, 0, len(ids))
	for _, id := range ids {
		record, err := s.GetComparison(ctx, id)
		if err != nil {
			return nil, err
		}
		comparisons = append(comparisons, record.Comparison)
	}
	return comparisons, nil
}

func (s *Store) listComparisonStacks(ctx context.Context, comparisonID uuid.UUID) ([]ComparisonStack, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, label, source, provider_id, model FROM comparison_stacks WHERE comparison_id = $1 ORDER BY position`, comparisonID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ComparisonStack])
}

// ListRunningComparisonIDs returns the comparisons still running, oldest first.
func (s *Store) ListRunningComparisonIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM comparisons WHERE status = 'running' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// ListNewestJobIDsWithText returns up to limit open jobs that have a
// description, the newest first: fresh inputs for a comparison.
func (s *Store) ListNewestJobIDsWithText(ctx context.Context, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM jobs WHERE closed_at IS NULL AND dismissed_at IS NULL AND btrim(description) <> ''
		ORDER BY first_seen_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func convertEmptyJSONToNull(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}
