package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OwnerProfile describes the candidate the hub works for, as markdown the
// owner writes: background, skills, positioning, location, preferences.
type OwnerProfile struct {
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) GetOwnerProfile(ctx context.Context) (OwnerProfile, error) {
	var profile OwnerProfile
	err := s.pool.QueryRow(ctx, `SELECT body, updated_at FROM owner_profile`).Scan(&profile.Body, &profile.UpdatedAt)
	return profile, err
}

// SaveOwnerProfile replaces the profile and records its before and after. A
// save that changes nothing records nothing.
func (s *Store) SaveOwnerProfile(ctx context.Context, actor Actor, body string) (OwnerProfile, error) {
	var profile OwnerProfile
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		var current string
		if err := tx.QueryRow(ctx, `SELECT id, body, updated_at FROM owner_profile FOR UPDATE`).Scan(&id, &current, &profile.UpdatedAt); err != nil {
			return err
		}
		profile.Body = current
		if body == current {
			return nil
		}

		if err := tx.QueryRow(ctx, `UPDATE owner_profile SET body = $2, updated_at = now() WHERE id = $1 RETURNING body, updated_at`, id, body).
			Scan(&profile.Body, &profile.UpdatedAt); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "owner_profile", entityID: id, operation: "update",
			before: map[string]string{"body": current}, after: map[string]string{"body": body},
		})
	})
	return profile, err
}
