package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const databaseUsage = "usage: hub-server database restore <dump>"

// runDatabaseCommand runs hub-server database <command> on the database the
// server owns. It needs none of the server's settings but where that
// database is, so it runs from a terminal without server.env.
func runDatabaseCommand(arguments []string, lookup func(string) string, out io.Writer) error {
	if len(arguments) != 2 || arguments[0] != "restore" {
		return errors.New(databaseUsage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if lookup("HUB_DATABASE_URL") != "" {
		fmt.Fprintln(out, "HUB_DATABASE_URL is set here: a server started with it uses that database, not the one restored now.")
	}
	engines, dir := parseDatabasePaths(lookup)
	return restoreDatabase(ctx, engines, dir, arguments[1], out)
}

// restoreDatabase replaces the cluster in dir with one restored from dump by
// the newest engine in engines, once the restored database migrates.
func restoreDatabase(ctx context.Context, engines, dir, dump string, out io.Writer) error {
	engine, err := postgresprocess.NewestEngine(engines)
	if err != nil {
		return err
	}
	replaced, err := postgresprocess.Restore(ctx, postgresprocess.Settings{Engine: engine, Dir: dir}, dump, migrateRestored, out)
	if errors.Is(err, postgresprocess.ErrLocked) {
		return fmt.Errorf("%w; stop the hub in Settings › Server first", err)
	}
	if err != nil {
		return fmt.Errorf("the restore failed, and the current database is as it was: %w", err)
	}
	fmt.Fprintf(out, "Restored %s into %s.\n", dump, dir)
	if replaced != "" {
		fmt.Fprintf(out, "The database it replaced is kept in %s; delete that folder once the hub runs well.\n", replaced)
	}
	fmt.Fprintln(out, "Start the hub in Settings › Server.")
	return nil
}

// migrateRestored checks that the restored database is the hub's, then
// migrates it, as the server's next start would.
func migrateRestored(ctx context.Context, databaseURL string) error {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	version, _, err := store.GetMigrationState(ctx, pool)
	if err != nil {
		return err
	}
	if version == 0 {
		return errors.New("the dump holds no hub database: none of the hub's migrations are in it")
	}
	return store.Migrate(ctx, pool)
}
