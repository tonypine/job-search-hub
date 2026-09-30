package store

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// notAGoodFit is the reason a card dismissed from the board carries, ahead
// of the owner's note.
const notAGoodFit = "not a good fit"

// DismissApplication takes a card off the board as not a good fit, with the
// owner's note when there is one, without moving it out of its phase. A card
// with a job dismisses the job, so the Jobs list and the board agree; a card
// for a company alone is dismissed itself.
func (s *Store) DismissApplication(ctx context.Context, actor Actor, id uuid.UUID, note string) (Application, error) {
	reason := notAGoodFit
	if note = strings.TrimSpace(note); note != "" {
		reason += ": " + note
	}
	return s.updateApplicationDismissal(ctx, actor, id, change{operation: "dismiss", after: map[string]string{"reason": reason}},
		func(tx pgx.Tx, jobID uuid.UUID) error {
			_, err := dismissJobsInTransaction(ctx, tx, actor, []uuid.UUID{jobID}, reason)
			return err
		},
		`UPDATE applications SET dismissed_at = COALESCE(dismissed_at, now()), dismissal_reason = $2, updated_at = now() WHERE id = $1`, reason)
}

// RestoreApplication puts a dismissed card back on the board, in the phase
// it left.
func (s *Store) RestoreApplication(ctx context.Context, actor Actor, id uuid.UUID) (Application, error) {
	return s.updateApplicationDismissal(ctx, actor, id, change{operation: "restore"},
		func(tx pgx.Tx, jobID uuid.UUID) error {
			_, err := restoreJobsInTransaction(ctx, tx, actor, []uuid.UUID{jobID})
			return err
		},
		`UPDATE applications SET dismissed_at = NULL, dismissal_reason = '', updated_at = now() WHERE id = $1`)
}

// updateApplicationDismissal changes a card's dismissal through its job with
// updateJob, or through companyUpdate when it has none, and records entry.
func (s *Store) updateApplicationDismissal(ctx context.Context, actor Actor, id uuid.UUID, entry change,
	updateJob func(pgx.Tx, uuid.UUID) error, companyUpdate string, arguments ...any) (Application, error) {
	var application Application
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		application, err = scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM applications WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if application.JobID != nil {
			err = updateJob(tx, *application.JobID)
		} else {
			_, err = tx.Exec(ctx, companyUpdate, append([]any{id}, arguments...)...)
		}
		if err != nil {
			return err
		}
		entry.entityType, entry.entityID = "application", id
		return insertChange(ctx, tx, actor, entry)
	})
	return application, err
}
