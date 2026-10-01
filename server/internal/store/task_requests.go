package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrTaskNotFound  = errors.New("task not found")
	ErrTaskNotQueued = errors.New("the task is not waiting to run")
)

// Kinds of work the phone can ask the Mac to do.
const (
	TaskFindJobs        = "find_jobs"
	TaskResearchCompany = "research_company"
	// TaskFixJob corrects a job's details from the owner's note, its input.
	TaskFixJob = "fix_job"
)

// Task statuses.
const (
	TaskQueued    = "queued"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"
)

// TaskRequest is work asked of the Mac, which is the only place agents run.
type TaskRequest struct {
	ID         uuid.UUID  `json:"id"`
	Kind       string     `json:"kind"`
	CompanyID  *uuid.UUID `json:"company_id,omitempty"`
	JobID      *uuid.UUID `json:"job_id,omitempty"`
	Input      string     `json:"input,omitempty"`
	Status     string     `json:"status"`
	Result     string     `json:"result,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

const taskColumns = `id, kind, company_id, job_id, input, status, result, created_at, started_at, finished_at`

func scanTask(row pgx.Row) (TaskRequest, error) {
	var task TaskRequest
	err := row.Scan(&task.ID, &task.Kind, &task.CompanyID, &task.JobID, &task.Input, &task.Status, &task.Result, &task.CreatedAt, &task.StartedAt, &task.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TaskRequest{}, ErrTaskNotFound
	}
	return task, err
}

// QueueTask asks the Mac for work: finding a company's jobs, or researching
// a company given by name or link.
func (s *Store) QueueTask(ctx context.Context, actor Actor, kind string, companyID *uuid.UUID, input string, deviceID *uuid.UUID) (TaskRequest, error) {
	input = strings.TrimSpace(input)
	switch {
	case kind == TaskFindJobs && companyID == nil:
		return TaskRequest{}, errors.New("finding jobs needs the company")
	case kind == TaskResearchCompany && input == "":
		return TaskRequest{}, errors.New("researching needs the company's name or link")
	case kind == TaskFixJob:
		return TaskRequest{}, errors.New("a job fix is queued with QueueJobFix")
	case kind != TaskFindJobs && kind != TaskResearchCompany:
		return TaskRequest{}, errors.New("kind must be find_jobs, research_company or fix_job")
	}
	if kind == TaskResearchCompany {
		companyID = nil
	}
	var task TaskRequest
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		task, err = scanTask(tx.QueryRow(ctx, `
			INSERT INTO task_requests (kind, company_id, input, device_id) VALUES ($1, $2, $3, $4) RETURNING `+taskColumns,
			kind, companyID, input, deviceID))
		if isForeignKeyViolation(err) {
			return ErrCompanyNotFound
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "task_request", entityID: task.ID, operation: "queue", after: map[string]string{"kind": kind}})
	})
	return task, err
}

// QueueJobFix asks the Mac to correct a job's details from the owner's note.
// The task carries the job's company, so its updates show with the company.
// A claimed fix starts running at once for the caller, so no other runner
// takes it: the Mac asking for its own fix runs it itself.
func (s *Store) QueueJobFix(ctx context.Context, actor Actor, jobID uuid.UUID, note string, deviceID *uuid.UUID, claimed bool) (TaskRequest, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return TaskRequest{}, errors.New("a fix needs a note saying what's wrong")
	}
	var task TaskRequest
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		task, err = scanTask(tx.QueryRow(ctx, `
			INSERT INTO task_requests (kind, company_id, job_id, input, device_id, status, started_at)
			SELECT $1, jobs.company_id, jobs.id, $3, $4, CASE WHEN $5 THEN 'running' ELSE 'queued' END, CASE WHEN $5 THEN now() END
			FROM jobs WHERE jobs.id = $2 RETURNING `+taskColumns,
			TaskFixJob, jobID, note, deviceID, claimed))
		if errors.Is(err, ErrTaskNotFound) {
			return ErrJobNotFound
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "task_request", entityID: task.ID, operation: "queue", after: map[string]string{"kind": TaskFixJob}})
	})
	return task, err
}

// ListTasks returns tasks with the status, or all when it is empty, oldest
// first.
func (s *Store) ListTasks(ctx context.Context, status string, limit int) ([]TaskRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskColumns+` FROM task_requests WHERE $1 = '' OR status = $1 ORDER BY created_at LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TaskRequest, error) { return scanTask(row) })
}

// ClaimTask marks a queued task as running, so no second runner takes it.
func (s *Store) ClaimTask(ctx context.Context, id uuid.UUID) (TaskRequest, error) {
	task, err := scanTask(s.pool.QueryRow(ctx, `
		UPDATE task_requests SET status = 'running', started_at = now() WHERE id = $1 AND status = 'queued' RETURNING `+taskColumns, id))
	if errors.Is(err, ErrTaskNotFound) {
		if _, lookupErr := scanTask(s.pool.QueryRow(ctx, `SELECT `+taskColumns+` FROM task_requests WHERE id = $1`, id)); lookupErr == nil {
			return TaskRequest{}, ErrTaskNotQueued
		}
	}
	return task, err
}

// FinishTask records how a running task ended; a company it found or added
// is kept with it.
func (s *Store) FinishTask(ctx context.Context, id uuid.UUID, succeeded bool, result string, companyID *uuid.UUID) (TaskRequest, error) {
	status := TaskFailed
	if succeeded {
		status = TaskSucceeded
	}
	task, err := scanTask(s.pool.QueryRow(ctx, `
		UPDATE task_requests SET status = $2, result = $3, company_id = coalesce(company_id, $4), finished_at = now()
		WHERE id = $1 AND status = 'running' RETURNING `+taskColumns, id, status, strings.TrimSpace(result), companyID))
	if errors.Is(err, ErrTaskNotFound) {
		if _, lookupErr := scanTask(s.pool.QueryRow(ctx, `SELECT `+taskColumns+` FROM task_requests WHERE id = $1`, id)); lookupErr == nil {
			return TaskRequest{}, errors.New("the task is not running")
		}
	}
	return task, err
}
