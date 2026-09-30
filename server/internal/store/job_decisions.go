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
			if decision == JobDecisionPursue {
				if _, _, err := addApplicationInTransaction(ctx, tx, actor, ApplicationInput{JobID: &jobID}); err != nil {
					return err
				}
			}
			if err := recordJobDecision(ctx, tx, jobID, decision, reason); err != nil {
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

func scanJobDecision(row pgx.Row) (JobDecision, error) {
	var decision JobDecision
	err := row.Scan(&decision.JobID, &decision.Decision, &decision.Reason, &decision.DecidedAt)
	return decision, err
}

// recordJobDecision makes decision the job's latest one.
func recordJobDecision(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, decision, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO job_decisions (job_id, decision, reason) VALUES ($1, $2, $3)
		ON CONFLICT (job_id) DO UPDATE SET decision = EXCLUDED.decision, reason = EXCLUDED.reason, decided_at = now()`,
		jobID, decision, reason)
	return err
}
