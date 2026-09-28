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

const foreignKeyViolation = "23503"

// isForeignKeyViolation reports whether err is Postgres refusing a row that
// points at a missing parent, such as an unknown company.
func isForeignKeyViolation(err error) bool {
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) && postgresErr.Code == foreignKeyViolation
}

type ActorKind string

const (
	ActorOwner    ActorKind = "owner"
	ActorAgentRun ActorKind = "agent_run"
)

// Actor is who made a write: the owner, or one agent run.
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
