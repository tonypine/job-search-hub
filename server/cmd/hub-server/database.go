package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// hubDatabase is the hub's Postgres, migrated: one the server runs itself
// when no HUB_DATABASE_URL is set, or the one it names.
type hubDatabase struct {
	pool *pgxpool.Pool
	// url reaches the database, for pg_dump, which dumps it with pgDump.
	url    string
	pgDump string
	// cluster is the Postgres the server runs, or nil for one it doesn't.
	cluster *postgresprocess.Cluster
}

// openDatabase starts the server's own Postgres from the newest engine, or
// waits for the one settings name, then migrates it. The caller closes it.
func openDatabase(ctx context.Context, settings config) (*hubDatabase, error) {
	database := &hubDatabase{url: settings.databaseURL, pgDump: settings.pgDump}
	if settings.databaseURL == "" {
		engine, err := postgresprocess.NewestEngine(settings.postgresEngines)
		if err != nil {
			return nil, err
		}
		cluster, err := postgresprocess.Start(ctx, postgresprocess.Settings{Engine: engine, Dir: settings.postgresDir})
		if err != nil {
			return nil, fmt.Errorf("start the database: %w", err)
		}
		database.cluster = cluster
		database.url = cluster.URL()
		// The engine's pg_dump always matches its server's version.
		if database.pgDump == "" {
			database.pgDump = filepath.Join(engine, "bin", "pg_dump")
		}
	}
	if database.pgDump == "" {
		database.pgDump = "pg_dump"
	}

	pool, err := pgxpool.New(ctx, database.url)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("configure the database pool: %w", err)
	}
	database.pool = pool
	// A Postgres the server started is up already. One it didn't may still
	// be starting: at login, launchd starts the server before Docker Desktop
	// has Postgres up. Only the one the server runs is dumped before it is
	// migrated, with the engine's own pg_dump.
	if database.cluster == nil {
		if err := waitForDatabase(ctx, pool, startupDatabaseWait); err != nil {
			database.Close()
			return nil, err
		}
	} else if err := dumpBeforeMigrating(ctx, database, settings.backupsDir); err != nil {
		database.Close()
		return nil, err
	}
	if err := store.Migrate(ctx, pool); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

// dumpBeforeMigrating dumps the database into folder when migrations are
// pending, so one that goes wrong can be undone. A new database has nothing
// to keep. Without the dump, the migrations wait.
func dumpBeforeMigrating(ctx context.Context, database *hubDatabase, folder string) error {
	version, pending, err := store.GetMigrationState(ctx, database.pool)
	if err != nil {
		return err
	}
	if !pending || version == 0 {
		return nil
	}
	path, err := databasebackup.DumpBeforeMigration(ctx, database.pgDump, database.url, folder, version)
	if err != nil {
		return fmt.Errorf("dump the database before migrating it: %w", err)
	}
	slog.Info("database dumped before migrating", "file", path)
	return nil
}

// Exited closes when the Postgres the server runs stops by itself, and never
// for one it doesn't run.
func (database *hubDatabase) Exited() <-chan struct{} {
	if database.cluster == nil {
		return nil
	}
	return database.cluster.Exited()
}

// ExitError says why the Postgres the server runs stopped by itself.
func (database *hubDatabase) ExitError() error {
	if database.cluster == nil {
		return nil
	}
	return database.cluster.ExitError()
}

// Close closes the pool, then stops the Postgres the server runs, so no
// connection is cut by its shutdown.
func (database *hubDatabase) Close() {
	if database.pool != nil {
		database.pool.Close()
	}
	if database.cluster != nil {
		database.cluster.Stop()
	}
}
