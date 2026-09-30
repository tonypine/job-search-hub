package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// modelWorkSettingsID stands for the one model_work_settings row in the
// change log.
var modelWorkSettingsID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// GetModelWorkPaused says whether background model work is paused.
func (s *Store) GetModelWorkPaused(ctx context.Context) (bool, error) {
	var paused bool
	err := s.pool.QueryRow(ctx, `SELECT paused FROM model_work_settings`).Scan(&paused)
	return paused, err
}

// SetModelWorkPaused pauses or resumes background model work.
func (s *Store) SetModelWorkPaused(ctx context.Context, actor Actor, paused bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE model_work_settings SET paused = $1, updated_at = now()`, paused); err != nil {
			return err
		}
		operation := "resume"
		if paused {
			operation = "pause"
		}
		return insertChange(ctx, tx, actor, change{entityType: "model_work", entityID: modelWorkSettingsID, operation: operation, after: map[string]bool{"paused": paused}})
	})
}
