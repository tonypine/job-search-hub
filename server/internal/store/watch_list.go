package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const foreignKeyViolation = "23503"

type WatchedCompany struct {
	Company      Company   `json:"company"`
	WatchedSince time.Time `json:"watched_since"`
}

// AddToWatchList puts the company on the watch list. A company already on it
// keeps its original entry; added reports whether a new entry was made.
func (s *Store) AddToWatchList(ctx context.Context, actor Actor, companyID uuid.UUID) (time.Time, bool, error) {
	var watchedSince time.Time
	added := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var entryID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO watch_list_entries (company_id) VALUES ($1)
			ON CONFLICT (company_id) WHERE removed_at IS NULL DO NOTHING
			RETURNING id, added_at`, companyID).Scan(&entryID, &watchedSince)
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.QueryRow(ctx, `SELECT added_at FROM watch_list_entries WHERE company_id = $1 AND removed_at IS NULL`, companyID).
				Scan(&watchedSince)
		}
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) && postgresErr.Code == foreignKeyViolation {
			return ErrCompanyNotFound
		}
		if err != nil {
			return err
		}
		added = true
		return insertChange(ctx, tx, actor, change{
			entityType: "watch_list_entry", entityID: entryID, operation: "add", after: map[string]any{"company_id": companyID},
		})
	})
	return watchedSince, added, err
}

// RemoveFromWatchList ends the company's active entry, which keeps its dates.
// removed is false when the company was not on the watch list.
func (s *Store) RemoveFromWatchList(ctx context.Context, actor Actor, companyID uuid.UUID) (bool, error) {
	removed := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var entryID uuid.UUID
		err := tx.QueryRow(ctx, `
			UPDATE watch_list_entries SET removed_at = now()
			WHERE company_id = $1 AND removed_at IS NULL
			RETURNING id`, companyID).Scan(&entryID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		removed = true
		return insertChange(ctx, tx, actor, change{
			entityType: "watch_list_entry", entityID: entryID, operation: "remove", before: map[string]any{"company_id": companyID},
		})
	})
	return removed, err
}

// ListWatchList returns the watched companies, most recently added first.
func (s *Store) ListWatchList(ctx context.Context) ([]WatchedCompany, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+companyColumns+`, watched.added_at
		FROM companies
		JOIN (SELECT company_id, added_at FROM watch_list_entries WHERE removed_at IS NULL) AS watched
			ON watched.company_id = companies.id
		ORDER BY watched.added_at DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (WatchedCompany, error) {
		var watched WatchedCompany
		company, err := scanCompany(row, &watched.WatchedSince)
		watched.Company = company
		return watched, err
	})
}

// GetWatchedSince returns when the company was added to the watch list, or
// nil when it is not on it.
func (s *Store) GetWatchedSince(ctx context.Context, companyID uuid.UUID) (*time.Time, error) {
	var watchedSince time.Time
	err := s.pool.QueryRow(ctx, `SELECT added_at FROM watch_list_entries WHERE company_id = $1 AND removed_at IS NULL`, companyID).
		Scan(&watchedSince)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &watchedSince, nil
}
