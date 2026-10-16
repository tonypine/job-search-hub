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

const (
	JobDecisionPursue = "pursue"
	JobDecisionSkip   = "skip"
	JobDecisionLater  = "later"
)

// JobDecision is the owner's latest verdict on a job.
type JobDecision struct {
	JobID     uuid.UUID `json:"job_id"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason,omitempty"`
	DecidedAt time.Time `json:"decided_at"`
}

// DecideJob records the owner's decision on a job and acts on it: pursue
// puts it on the pipeline, skip dismisses it with the reason, and later
// only records. Pursuing or leaving for later a dismissed job restores it.
func (s *Store) DecideJob(ctx context.Context, actor Actor, jobID uuid.UUID, decision, reason string) (JobDecision, error) {
	reason = strings.TrimSpace(reason)
	if decision != JobDecisionPursue && decision != JobDecisionSkip && decision != JobDecisionLater {
		return JobDecision{}, fmt.Errorf("decision must be %s, %s or %s", JobDecisionPursue, JobDecisionSkip, JobDecisionLater)
	}
	var recorded JobDecision
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var dismissed bool
		err := tx.QueryRow(ctx, `SELECT dismissed_at IS NOT NULL FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&dismissed)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJobNotFound
		}
		if err != nil {
			return err
		}
		if decision == JobDecisionSkip {
			if _, err := dismissJobsInTransaction(ctx, tx, actor, []uuid.UUID{jobID}, reason); err != nil {
				return err
			}
		} else {
			if dismissed {
				if _, err := restoreJobsInTransaction(ctx, tx, actor, []uuid.UUID{jobID}); err != nil {
					return err
				}
			}
			var addedApplicationID *uuid.UUID
			if decision == JobDecisionPursue {
				application, created, err := addApplicationInTransaction(ctx, tx, actor, ApplicationInput{JobID: &jobID})
				if err != nil {
					return err
				}
				if created {
					addedApplicationID = &application.ID
				}
			}
			if err := recordJobDecision(ctx, tx, jobID, decision, reason, addedApplicationID); err != nil {
				return err
			}
		}
		if err := insertChange(ctx, tx, actor, change{entityType: "job", entityID: jobID, operation: "decide",
			after: map[string]string{"decision": decision, "reason": reason}}); err != nil {
			return err
		}
		recorded, err = scanJobDecision(tx.QueryRow(ctx, `SELECT `+jobDecisionColumns+` FROM job_decisions WHERE job_id = $1`, jobID))
		return err
	})
	return recorded, err
}

// ClearedJobDecision is what taking back a decision did.
type ClearedJobDecision struct {
	// Decision is the decision taken back; empty when the job was undecided.
	Decision string `json:"decision,omitempty"`
	// RemovedApplicationID is the card the pursue put on the pipeline, which
	// left it with the pursue.
	RemovedApplicationID *uuid.UUID `json:"removed_application_id,omitempty"`
	// KeptApplicationID is the card the pursue put on the pipeline, which
	// stays because it changed since: it moved phase, or got a follow-up or
	// notes.
	KeptApplicationID *uuid.UUID `json:"kept_application_id,omitempty"`
}

// ClearJobDecision takes back the owner's decision on a job, which leaves it
// undecided: a skipped job is restored, and a job left for later goes back
// among the undecided. A pursued job leaves the pipeline when the pursue put
// it there and its card hasn't changed since; a card that was there before
// the pursue, or changed after it, stays. Clearing an undecided job changes
// nothing.
func (s *Store) ClearJobDecision(ctx context.Context, actor Actor, jobID uuid.UUID) (ClearedJobDecision, error) {
	var result ClearedJobDecision
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var dismissed bool
		err := tx.QueryRow(ctx, `SELECT dismissed_at IS NOT NULL FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&dismissed)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJobNotFound
		}
		if err != nil {
			return err
		}
		var addedApplicationID *uuid.UUID
		cleared, err := scanJobDecision(tx.QueryRow(ctx, `DELETE FROM job_decisions WHERE job_id = $1 RETURNING `+jobDecisionColumns+`, added_application_id`, jobID),
			&addedApplicationID)
		if errors.Is(err, pgx.ErrNoRows) {
			if !dismissed {
				return nil
			}
		} else if err != nil {
			return err
		}
		result.Decision = cleared.Decision
		if dismissed {
			if _, err := restoreJobsInTransaction(ctx, tx, actor, []uuid.UUID{jobID}); err != nil {
				return err
			}
		}
		if addedApplicationID != nil {
			removed, err := deleteUnchangedApplication(ctx, tx, actor, *addedApplicationID)
			if err != nil {
				return err
			}
			if removed {
				result.RemovedApplicationID = addedApplicationID
			} else {
				result.KeptApplicationID = addedApplicationID
			}
		}
		return insertChange(ctx, tx, actor, change{entityType: "job", entityID: jobID, operation: "undecide",
			before: map[string]string{"decision": cleared.Decision, "reason": cleared.Reason}})
	})
	return result, err
}

// deleteUnchangedApplication takes the card off the pipeline unless it
// changed since it was added, which every move, note, follow-up and contact
// does; removed reports whether it went.
func deleteUnchangedApplication(ctx context.Context, tx pgx.Tx, actor Actor, id uuid.UUID) (removed bool, err error) {
	application, err := scanApplication(tx.QueryRow(ctx, `
		DELETE FROM applications WHERE id = $1 AND updated_at = created_at RETURNING `+applicationColumns, id))
	if errors.Is(err, ErrApplicationNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, insertChange(ctx, tx, actor, change{entityType: "application", entityID: id, operation: "delete", before: application})
}

// GetJobDecision returns the job's latest decision; nil when undecided.
func (s *Store) GetJobDecision(ctx context.Context, jobID uuid.UUID) (*JobDecision, error) {
	decision, err := scanJobDecision(s.pool.QueryRow(ctx, `SELECT `+jobDecisionColumns+` FROM job_decisions WHERE job_id = $1`, jobID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &decision, nil
}

const jobDecisionColumns = `job_id, decision, reason, decided_at`

func scanJobDecision(row pgx.Row, extra ...any) (JobDecision, error) {
	var decision JobDecision
	err := row.Scan(append([]any{&decision.JobID, &decision.Decision, &decision.Reason, &decision.DecidedAt}, extra...)...)
	return decision, err
}

// recordJobDecision makes decision the job's latest one. addedApplicationID
// is the card a pursue put on the pipeline, if it put one; pursuing a
// pursued job again keeps the card the first pursue added.
func recordJobDecision(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, decision, reason string, addedApplicationID *uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO job_decisions (job_id, decision, reason, added_application_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (job_id) DO UPDATE SET decision = EXCLUDED.decision, reason = EXCLUDED.reason, decided_at = now(),
			added_application_id = CASE WHEN job_decisions.decision = $5 AND EXCLUDED.decision = $5
				THEN COALESCE(EXCLUDED.added_application_id, job_decisions.added_application_id)
				ELSE EXCLUDED.added_application_id END`,
		jobID, decision, reason, addedApplicationID, JobDecisionPursue)
	return err
}
