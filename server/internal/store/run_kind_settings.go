package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrRunKindNotFound means no kind of run has the id.
var ErrRunKindNotFound = errors.New("run kind not found")

// ErrRunKindNotPausable refuses to pause a kind the hub never starts by
// itself: it runs only when the owner asks.
var ErrRunKindNotPausable = errors.New("this kind runs only when you ask, so it can't be paused")

// modelWorkSettingsID stands for the hub's model work in the change log,
// where each kind's pause and resume is recorded.
var modelWorkSettingsID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// ListAutomaticRunKindIDs are the ids of the kinds the hub starts by itself,
// the ones that can be paused.
func ListAutomaticRunKindIDs() []string {
	var ids []string
	for _, kind := range RunKinds {
		if kind.Automatic {
			ids = append(ids, kind.ID)
		}
	}
	return ids
}

// GetRunKindPauses says, for every kind the hub starts by itself, whether it
// is paused. A kind with no saved pause is paused, as on a new hub.
func (s *Store) GetRunKindPauses(ctx context.Context) (map[string]bool, error) {
	pauses := map[string]bool{}
	for _, kind := range ListAutomaticRunKindIDs() {
		pauses[kind] = true
	}
	rows, err := s.pool.Query(ctx, `SELECT kind, paused FROM run_kind_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var paused bool
		if err := rows.Scan(&kind, &paused); err != nil {
			return nil, err
		}
		if _, automatic := pauses[kind]; automatic {
			pauses[kind] = paused
		}
	}
	return pauses, rows.Err()
}

// SetRunKindsPaused pauses or resumes each of kinds, and records each change
// with its actor. It changes none when one of them is unknown
// (ErrRunKindNotFound) or runs only when the owner asks
// (ErrRunKindNotPausable).
func (s *Store) SetRunKindsPaused(ctx context.Context, actor Actor, kinds []string, paused bool) error {
	for _, id := range kinds {
		kind, found := GetRunKind(id)
		if !found {
			return ErrRunKindNotFound
		}
		if !kind.Automatic {
			return ErrRunKindNotPausable
		}
	}
	operation := "resume"
	if paused {
		operation = "pause"
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, kind := range kinds {
			if _, err := tx.Exec(ctx, `
				INSERT INTO run_kind_settings (kind, paused) VALUES ($1, $2)
				ON CONFLICT (kind) DO UPDATE SET paused = excluded.paused, updated_at = now()`, kind, paused); err != nil {
				return err
			}
			after := map[string]any{"kind": kind, "paused": paused}
			if err := insertChange(ctx, tx, actor, change{entityType: "model_work", entityID: modelWorkSettingsID, operation: operation, after: after}); err != nil {
				return err
			}
		}
		return nil
	})
}
