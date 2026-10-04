package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FreshMatch is a job judged a strong match while it is still fresh, that
// the owner hasn't been told about.
type FreshMatch struct {
	JobID       uuid.UUID
	CompanyID   *uuid.UUID
	JobTitle    string
	CompanyName *string
	// PublishedAt is when the board says the job was posted, nil when it
	// doesn't say.
	PublishedAt *time.Time
	FirstSeenAt time.Time
	// Reason is why the job's brief judged it a strong match.
	Reason string
}

// ListFreshMatchesToTell returns the open, undismissed jobs posted after
// postedAfter (or first seen then, when the board gives no date) whose brief,
// the full one if written, judges them a strong match, and that the owner
// hasn't decided on, put on the board or been told about: newest first.
func (s *Store) ListFreshMatchesToTell(ctx context.Context, postedAfter time.Time) ([]FreshMatch, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT jobs.id, jobs.company_id, jobs.title, COALESCE(companies.name, NULLIF(jobs.company_name, '')),
		       jobs.published_at, jobs.first_seen_at, brief.reason
		FROM jobs
		JOIN LATERAL (
			SELECT match, reason FROM job_briefs WHERE job_briefs.job_id = jobs.id ORDER BY tier = 'full' DESC LIMIT 1
		) brief ON brief.match = 'strong'
		LEFT JOIN companies ON companies.id = jobs.company_id
		WHERE jobs.closed_at IS NULL AND jobs.dismissed_at IS NULL AND jobs.fresh_match_told_at IS NULL
		  AND COALESCE(jobs.published_at, jobs.first_seen_at) > $1
		  AND NOT EXISTS (SELECT 1 FROM applications WHERE applications.job_id = jobs.id)
		  AND NOT EXISTS (SELECT 1 FROM job_decisions WHERE job_decisions.job_id = jobs.id)
		ORDER BY COALESCE(jobs.published_at, jobs.first_seen_at) DESC`, postedAfter)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (FreshMatch, error) {
		var match FreshMatch
		err := row.Scan(&match.JobID, &match.CompanyID, &match.JobTitle, &match.CompanyName, &match.PublishedAt, &match.FirstSeenAt, &match.Reason)
		return match, err
	})
}

// MarkFreshMatchTold records that the owner was told of the job as a fresh
// match. It is a notice about the hub's own records, so it writes no change.
func (s *Store) MarkFreshMatchTold(ctx context.Context, jobID uuid.UUID, toldAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE jobs SET fresh_match_told_at = $2 WHERE id = $1`, jobID, toldAt)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrJobNotFound
	}
	return err
}
