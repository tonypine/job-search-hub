package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DueFollowUp is a card whose follow-up has fallen due and that the owner
// hasn't been told about for this due time.
type DueFollowUp struct {
	ApplicationID    uuid.UUID
	JobID            *uuid.UUID
	CompanyID        *uuid.UUID
	JobTitle         *string
	CompanyName      *string
	PhaseName        string
	PhaseEnteredAt   time.Time
	LastFollowedUpAt *time.Time
	DueAt            time.Time
}

// ListFollowUpsToRemind returns the open cards on the board whose follow-up
// falls due before dueBefore and hasn't been reminded for that due time,
// soonest due first. Closed and dismissed cards never fall due.
func (s *Store) ListFollowUpsToRemind(ctx context.Context, dueBefore time.Time) ([]DueFollowUp, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT applications.id, applications.job_id, applications.company_id, jobs.title, COALESCE(companies.name, NULLIF(jobs.company_name, '')),
		       pipeline_phases.name, applications.phase_entered_at, applications.last_followed_up_at, `+cardFollowUpDueAt+` AS due_at
		FROM applications
		JOIN pipeline_phases ON pipeline_phases.id = applications.phase_id
		LEFT JOIN jobs ON jobs.id = applications.job_id
		LEFT JOIN companies ON companies.id = applications.company_id
		WHERE NOT pipeline_phases.is_closed AND pipeline_phases.follow_up_days IS NOT NULL AND `+cardDismissedAt+` IS NULL
		  AND `+cardFollowUpDueAt+` < $1
		  AND (applications.follow_up_reminded_at IS NULL OR applications.follow_up_reminded_at < `+cardFollowUpDueAt+`)
		ORDER BY due_at`, dueBefore)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (DueFollowUp, error) {
		var due DueFollowUp
		err := row.Scan(&due.ApplicationID, &due.JobID, &due.CompanyID, &due.JobTitle, &due.CompanyName,
			&due.PhaseName, &due.PhaseEnteredAt, &due.LastFollowedUpAt, &due.DueAt)
		return due, err
	})
}

// MarkFollowUpReminded records that the owner was told of the card's
// follow-up due at dueAt. It is a notice about the hub's own records, so it
// writes no change.
func (s *Store) MarkFollowUpReminded(ctx context.Context, id uuid.UUID, dueAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE applications SET follow_up_reminded_at = $2 WHERE id = $1`, id, dueAt)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrApplicationNotFound
	}
	return err
}
