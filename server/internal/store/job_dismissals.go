package store

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	dismissJobsUpdate = `UPDATE jobs SET dismissed_at = COALESCE(dismissed_at, now()), dismissal_reason = $2
		WHERE id = ANY($1) RETURNING ` + jobColumns
	restoreJobsUpdate = `UPDATE jobs SET dismissed_at = NULL, dismissal_reason = ''
		WHERE id = ANY($1) RETURNING ` + jobColumns
)

// DismissJobs dismisses each job with the owner's reason, which may be
// empty: it leaves the jobs list, job facts and the board until restored.
// Dismissing a dismissed job replaces its reason. An unknown id dismisses
// nothing.
func (s *Store) DismissJobs(ctx context.Context, actor Actor, jobIDs []uuid.UUID, reason string) ([]Job, error) {
	var jobs []Job
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		jobs, err = dismissJobsInTransaction(ctx, tx, actor, jobIDs, reason)
		return err
	})
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// RestoreJobs brings dismissed jobs back to the jobs list, and their cards
// back to the board. An unknown id restores nothing.
func (s *Store) RestoreJobs(ctx context.Context, actor Actor, jobIDs []uuid.UUID) ([]Job, error) {
	var jobs []Job
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		jobs, err = restoreJobsInTransaction(ctx, tx, actor, jobIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func dismissJobsInTransaction(ctx context.Context, tx pgx.Tx, actor Actor, jobIDs []uuid.UUID, reason string) ([]Job, error) {
	return updateJobDismissals(ctx, tx, actor, jobIDs, change{operation: "dismiss", after: map[string]string{"reason": reason}}, dismissJobsUpdate, reason)
}

func restoreJobsInTransaction(ctx context.Context, tx pgx.Tx, actor Actor, jobIDs []uuid.UUID) ([]Job, error) {
	return updateJobDismissals(ctx, tx, actor, jobIDs, change{operation: "restore"}, restoreJobsUpdate)
}

// updateJobDismissals runs update, whose first argument is the job ids, and
// records entry as each job's change. Every job named must exist.
func updateJobDismissals(ctx context.Context, tx pgx.Tx, actor Actor, jobIDs []uuid.UUID, entry change, update string, arguments ...any) ([]Job, error) {
	uniqueIDs := slices.Compact(slices.SortedFunc(slices.Values(jobIDs), func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) }))
	if len(uniqueIDs) == 0 {
		return nil, errors.New("name at least one job")
	}
	rows, err := tx.Query(ctx, update, append([]any{uniqueIDs}, arguments...)...)
	if err != nil {
		return nil, err
	}
	jobs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Job, error) { return scanJob(row) })
	if err != nil {
		return nil, err
	}
	if len(jobs) != len(uniqueIDs) {
		return nil, ErrJobNotFound
	}
	for _, job := range jobs {
		entry.entityType, entry.entityID, entry.sourceURL = "job", job.ID, job.URL
		if err := insertChange(ctx, tx, actor, entry); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}
