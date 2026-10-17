package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrTaskNotFound   = errors.New("task not found")
	ErrTaskNotQueued  = errors.New("the task is not waiting to run")
	ErrTaskNotRunning = errors.New("the task is not running")
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
	// RunID is the id of the agent run an owner's start of the task's kind
	// answered with, which the run takes once the Mac starts it; Model is
	// the Claude model chosen for that run only, empty for its kind's.
	RunID *uuid.UUID `json:"run_id,omitempty"`
	Model string     `json:"model,omitempty"`
}

const taskColumns = `id, kind, company_id, job_id, input, status, result, created_at, started_at, finished_at, run_id, model`

func scanTask(row pgx.Row) (TaskRequest, error) {
	var task TaskRequest
	err := row.Scan(&task.ID, &task.Kind, &task.CompanyID, &task.JobID, &task.Input, &task.Status, &task.Result, &task.CreatedAt, &task.StartedAt,
		&task.FinishedAt, &task.RunID, &task.Model)
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

// taskKindsByAgentRunKind are the tasks the Mac runs each agent kind
// through.
var taskKindsByAgentRunKind = map[string]string{
	AgentRunKindJobFix:        TaskFixJob,
	AgentRunKindCompanyTriage: TaskResearchCompany,
	AgentRunKindJobFinder:     TaskFindJobs,
}

// NewAgentRunTask is an agent run the owner started for a job or a company,
// queued for the Mac: a job fix with its note, or a company's research or
// job finding. RunID is the id the run will be recorded under; Model is the
// Claude model chosen for it, empty for its kind's.
type NewAgentRunTask struct {
	Kind      string
	SubjectID uuid.UUID
	Note      string
	RunID     uuid.UUID
	Model     string
}

// QueueAgentRunTask asks the Mac to run an agent the owner started. A job's
// fix carries the job and its company; a company's research names the
// company by its domain, else its name, as researching a company does.
func (s *Store) QueueAgentRunTask(ctx context.Context, actor Actor, input NewAgentRunTask, deviceID *uuid.UUID) (TaskRequest, error) {
	taskKind, found := taskKindsByAgentRunKind[input.Kind]
	if !found {
		return TaskRequest{}, fmt.Errorf("%s doesn't run on the Mac", input.Kind)
	}
	note := strings.TrimSpace(input.Note)
	if taskKind == TaskFixJob && note == "" {
		return TaskRequest{}, errors.New("a fix needs a note saying what's wrong")
	}
	var task TaskRequest
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if taskKind == TaskFixJob {
			task, err = scanTask(tx.QueryRow(ctx, `
				INSERT INTO task_requests (kind, company_id, job_id, input, device_id, run_id, model)
				SELECT $1, jobs.company_id, jobs.id, $3, $4, $5, $6 FROM jobs WHERE jobs.id = $2 RETURNING `+taskColumns,
				taskKind, input.SubjectID, note, deviceID, input.RunID, input.Model))
			if errors.Is(err, ErrTaskNotFound) {
				return ErrJobNotFound
			}
		} else {
			task, err = scanTask(tx.QueryRow(ctx, `
				INSERT INTO task_requests (kind, company_id, input, device_id, run_id, model)
				SELECT $1, companies.id, CASE WHEN $1 = 'research_company' THEN coalesce(NULLIF(companies.domain, ''), companies.name) ELSE '' END,
					$3, $4, $5
				FROM companies WHERE companies.id = $2 RETURNING `+taskColumns,
				taskKind, input.SubjectID, deviceID, input.RunID, input.Model))
			if errors.Is(err, ErrTaskNotFound) {
				return ErrCompanyNotFound
			}
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "task_request", entityID: task.ID, operation: "queue", after: map[string]string{"kind": taskKind}})
	})
	return task, err
}

// StartedAgentRun is the id and model an owner's start gave an agent run.
type StartedAgentRun struct {
	ID    uuid.UUID
	Model string
}

// FindStartedAgentRun is the run the owner started of kind for the subject,
// which a runner has claimed and not started yet, the oldest first: the run
// starting now is that one.
func (s *Store) FindStartedAgentRun(ctx context.Context, kind string, subjectID uuid.UUID) (StartedAgentRun, bool, error) {
	taskKind, found := taskKindsByAgentRunKind[kind]
	if !found {
		return StartedAgentRun{}, false, nil
	}
	var started StartedAgentRun
	err := s.pool.QueryRow(ctx, `
		SELECT run_id, model FROM task_requests
		WHERE status = 'running' AND run_id IS NOT NULL AND kind = $1
		  AND CASE WHEN kind = 'fix_job' THEN job_id ELSE company_id END = $2
		  AND NOT EXISTS (SELECT 1 FROM agent_runs WHERE agent_runs.id = task_requests.run_id)
		ORDER BY started_at, created_at LIMIT 1`, taskKind, subjectID).Scan(&started.ID, &started.Model)
	if errors.Is(err, pgx.ErrNoRows) {
		return StartedAgentRun{}, false, nil
	}
	return started, err == nil, err
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

// ReleaseTask puts a running task back in the queue, as when an install
// restarted the Mac app while it ran: the next runner claims it again.
func (s *Store) ReleaseTask(ctx context.Context, id uuid.UUID) (TaskRequest, error) {
	task, err := scanTask(s.pool.QueryRow(ctx, `
		UPDATE task_requests SET status = 'queued', started_at = NULL WHERE id = $1 AND status = 'running' RETURNING `+taskColumns, id))
	if errors.Is(err, ErrTaskNotFound) {
		if _, lookupErr := scanTask(s.pool.QueryRow(ctx, `SELECT `+taskColumns+` FROM task_requests WHERE id = $1`, id)); lookupErr == nil {
			return TaskRequest{}, ErrTaskNotRunning
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
