package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrGoogleNotConnected = errors.New("google is not connected")

// GoogleConnection is the owner's Google sign-in. RefreshToken is never
// serialized.
type GoogleConnection struct {
	Email               string     `json:"email"`
	RefreshToken        string     `json:"-"`
	Scopes              []string   `json:"scopes"`
	ConnectedAt         time.Time  `json:"connected_at"`
	NeedsReconnectSince *time.Time `json:"needs_reconnect_since,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
}

func (s *Store) GetGoogleConnection(ctx context.Context) (GoogleConnection, error) {
	var connection GoogleConnection
	err := s.pool.QueryRow(ctx, `
		SELECT email, refresh_token, scopes, connected_at, needs_reconnect_since, last_error FROM google_connection`).
		Scan(&connection.Email, &connection.RefreshToken, &connection.Scopes, &connection.ConnectedAt, &connection.NeedsReconnectSince, &connection.LastError)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoogleConnection{}, ErrGoogleNotConnected
	}
	return connection, err
}

// SaveGoogleConnection replaces the sign-in with a fresh one. The change log
// records who signed in and to what, never the token.
func (s *Store) SaveGoogleConnection(ctx context.Context, actor Actor, email, refreshToken string, scopes []string) (GoogleConnection, error) {
	var connection GoogleConnection
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO google_connection (email, refresh_token, scopes) VALUES ($1, $2, $3)
			ON CONFLICT (singleton) DO UPDATE SET
				email = EXCLUDED.email, refresh_token = EXCLUDED.refresh_token, scopes = EXCLUDED.scopes,
				connected_at = now(), needs_reconnect_since = NULL, last_error = ''
			RETURNING id, email, refresh_token, scopes, connected_at`, email, refreshToken, scopes).
			Scan(&id, &connection.Email, &connection.RefreshToken, &connection.Scopes, &connection.ConnectedAt)
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "google_connection", entityID: id, operation: "connect", after: map[string]any{"email": email, "scopes": scopes},
		})
	})
	return connection, err
}

// MarkGoogleConnectionRefused notes that Google refused the stored token, so
// the owner must sign in again.
func (s *Store) MarkGoogleConnectionRefused(ctx context.Context, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE google_connection SET needs_reconnect_since = COALESCE(needs_reconnect_since, now()), last_error = $1`, reason)
	return err
}
