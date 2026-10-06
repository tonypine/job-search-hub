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

	"github.com/tonypine/job-search-hub/server/migrations"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Migrate applies every pending migration.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	database := stdlib.OpenDBFromPool(pool)
	defer database.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	for _, result := range results {
		slog.Info("migration applied", "version", result.Source.Version)
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
