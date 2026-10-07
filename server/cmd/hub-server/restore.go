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

const databaseUsage = "usage: hub-server database restore <dump>, hub-server database import [--replace] <postgres-url>, hub-server database move-from-compose <server.env>, or hub-server database count-writes <migration>"

// runDatabaseCommand runs hub-server database <command> on the database the
// server owns. It needs none of the server's settings but where that
// database and its dumps are, so it runs from a terminal without server.env.
func runDatabaseCommand(arguments []string, lookup func(string) string, out io.Writer) error {
	var command func(ctx context.Context, engines, dir string) error
	switch {
	case len(arguments) == 2 && arguments[0] == "restore":
		command = func(ctx context.Context, engines, dir string) error {
			return restoreDatabase(ctx, engines, dir, arguments[1], out)
		}
	case len(arguments) > 1 && arguments[0] == "import":
		source, replace, ok := parseImportArguments(arguments[1:])
		if !ok {
			return errors.New(databaseUsage)
		}
		command = func(ctx context.Context, engines, dir string) error {
			if err := importDatabase(ctx, engines, dir, parseBackupsDir(lookup), source, replace, checkImported, out); err != nil {
				return err
			}
			fmt.Fprintln(out, "Remove HUB_DATABASE_URL from ~/.config/job-search-hub/server.env, then start the hub in Settings › Server.")
			return nil
		}
	case len(arguments) == 2 && arguments[0] == "move-from-compose":
		// Run by install-app.sh while the server is stopped, with server.env
		// loaded: its HUB_DATABASE_URL is the database to move.
		command = func(ctx context.Context, engines, dir string) error {
			move := databaseMove{engines: engines, dir: dir, backups: parseBackupsDir(lookup), isOld: isComposeDatabase, check: checkImported}
			_, err := move.run(ctx, arguments[1], out)
			return err
		}
	case len(arguments) == 2 && arguments[0] == "count-writes":
		// Run by hub-update while the server is stopped, to say what rolling
		// back to the dump taken before migrating from <migration> loses. It
		// prints JSON, and nothing else.
		command = func(ctx context.Context, engines, dir string) error {
			return countWritesSinceMigration(ctx, engines, dir, parseBackupsDir(lookup), arguments[1], out)
		}
	default:
		return errors.New(databaseUsage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if lookup("HUB_DATABASE_URL") != "" && arguments[0] != "move-from-compose" && arguments[0] != "count-writes" {
		fmt.Fprintln(out, "HUB_DATABASE_URL is set here: a server started with it uses that database, not the one the server owns.")
	}
	engines, dir := parseDatabasePaths(lookup)
	return command(ctx, engines, dir)
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
