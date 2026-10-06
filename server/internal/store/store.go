// Package store reads and writes the hub's Postgres database. Every write
// records a row in changes, in the same transaction, naming who made it.
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
	"github.com/tonypine/job-search-hub/server/migrations"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// DatabaseAheadError is a database migrated past the newest migration this
// build knows, by a newer release: the server can't serve it.
type DatabaseAheadError struct {
	// Version is the database's migration, and Newest this build's.
	Version, Newest int64
	// Release first migrated the database to Version; empty when no release
	// recorded it, as after a dev build.
	Release string
}

func (e *DatabaseAheadError) Error() string {
	install := "Install a newer release"
	if e.Release != "" {
		install = "Install " + e.Release + " or later"
	}
	return fmt.Sprintf("The database is at migration %d; this server knows up to %d. %s, or restore a backup from before it.", e.Version, e.Newest, install)
}

// Migrate applies every pending migration, and records this build's release
// as the first to know the newest one. It refuses, with a
// *DatabaseAheadError, a database a newer release migrated further.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	release := ""
	if buildinfo.IsRelease() {
		release = buildinfo.Version()
	}
	return migrate(ctx, pool, release)
}

func migrate(ctx context.Context, pool *pgxpool.Pool, release string) error {
	database := stdlib.OpenDBFromPool(pool)
	defer database.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("read the database's migration version: %w", err)
	}
	if newest := migrations.Newest(); version > newest {
		ahead := &DatabaseAheadError{Version: version, Newest: newest}
		// A database that far ahead has migration_releases; a row may be
		// missing.
		if err := pool.QueryRow(ctx, `SELECT release FROM migration_releases WHERE migration = $1`, version).Scan(&ahead.Release); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("read which release migrated the database", "error", err)
		}
		return ahead
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	for _, result := range results {
		slog.Info("migration applied", "version", result.Source.Version)
	}
	if release == "" {
		return nil
	}
	if _, err := pool.Exec(ctx, `INSERT INTO migration_releases (migration, release) VALUES ($1, $2) ON CONFLICT (migration) DO NOTHING`, migrations.Newest(), release); err != nil {
		return fmt.Errorf("record the release that migrated the database: %w", err)
	}
	return nil
}

// GetMigrationState is the newest migration applied to the database, 0 for a
// new one, and whether any migration is still to apply.
func GetMigrationState(ctx context.Context, pool *pgxpool.Pool) (version int64, pending bool, err error) {
	database := stdlib.OpenDBFromPool(pool)
	defer database.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files)
	if err != nil {
		return 0, false, fmt.Errorf("load migrations: %w", err)
	}
	if version, err = provider.GetDBVersion(ctx); err != nil {
		return 0, false, fmt.Errorf("read the database's migration version: %w", err)
	}
	if pending, err = provider.HasPending(ctx); err != nil {
		return 0, false, fmt.Errorf("check for pending migrations: %w", err)
	}
	return version, pending, nil
}

const (
	foreignKeyViolation = "23503"
	uniqueViolation     = "23505"
)

// isForeignKeyViolation reports whether err is Postgres refusing a row that
// points at a missing parent, such as an unknown company.
func isForeignKeyViolation(err error) bool {
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) && postgresErr.Code == foreignKeyViolation
}

// isUniqueViolation reports whether err is Postgres refusing a row that
// repeats a value a unique index holds, such as a phase name.
func isUniqueViolation(err error) bool {
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) && postgresErr.Code == uniqueViolation
}

type ActorKind string

const (
	ActorOwner    ActorKind = "owner"
	ActorAgentRun ActorKind = "agent_run"
	ActorSystem   ActorKind = "system"
)

// Actor is who made a write: the owner, one agent run, or the hub itself
// (the board poller).
type Actor struct {
	Kind       ActorKind
	AgentRunID uuid.UUID
}

type change struct {
	entityType string
	entityID   uuid.UUID
	operation  string
	before     any
	after      any
	sourceURL  string
}

func insertChange(ctx context.Context, tx pgx.Tx, actor Actor, entry change) error {
	agentRunID := uuid.NullUUID{UUID: actor.AgentRunID, Valid: actor.Kind == ActorAgentRun}
	_, err := tx.Exec(ctx, `
		INSERT INTO changes (actor_kind, agent_run_id, entity_type, entity_id, operation, before, after, source_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		actor.Kind, agentRunID, entry.entityType, entry.entityID, entry.operation, entry.before, entry.after, entry.sourceURL)
	if err != nil {
		return fmt.Errorf("record %s %s change: %w", entry.entityType, entry.operation, err)
	}
	return nil
}
