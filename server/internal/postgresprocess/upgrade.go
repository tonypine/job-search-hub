package postgresprocess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
)

// UpgradeError is why Start couldn't move an older major's cluster to the
// engine's major, and ran the older one with its own engine instead. The
// next Start tries again.
type UpgradeError struct {
	From, To int
	Err      error
}

func (upgradeError *UpgradeError) Error() string {
	return fmt.Sprintf("moving the database from Postgres %d to %d failed: %v", upgradeError.From, upgradeError.To, upgradeError.Err)
}

func (upgradeError *UpgradeError) Unwrap() error {
	return upgradeError.Err
}

// upgrade moves the cluster of major from in dir to a new one of major to,
// the engine's: it starts the old cluster with oldEngine, counts its tables'
// rows, dumps it with engine's pg_dump into backups, and stops it. Then it
// builds the new cluster in <to>.partial, after removing one a killed upgrade
// left, restores the dump with engine's pg_restore, checks that every table
// has the rows it had, and renames it to <to>. The old cluster stays. On a
// failure, no <to>.partial is left.
func upgrade(ctx context.Context, oldEngine, engine, dir, backups, from, to string) (err error) {
	oldDataDir := filepath.Join(dir, from)
	dataDir := filepath.Join(dir, to)
	partial := dataDir + ".partial"
	slog.Info("moving the database to a new Postgres major", "from", from, "to", to)

	// A killed server may have left its Postgres running on the old cluster,
	// and a killed upgrade its own on the partial one; either holds the socket.
	if err := stopOrphan(ctx, oldEngine, oldDataDir, socketLockPath(dir)); err != nil {
		return err
	}
	if err := stopOrphan(ctx, engine, partial, socketLockPath(dir)); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if removeErr := os.RemoveAll(partial); removeErr != nil {
				slog.Warn("the unfinished cluster is left; the next start removes it", "data", partial, "error", removeErr)
			}
		}
	}()
	dump, counts, err := dumpOlder(ctx, oldEngine, engine, dir, oldDataDir, backups, from, to)
	if err != nil {
		return err
	}
	slog.Info("database dumped before moving it to a new Postgres major", "file", dump)
	if err := createCluster(ctx, engine, partial); err != nil {
		return err
	}
	check := func(ctx context.Context, databaseURL string) error {
		restored, err := countRows(ctx, databaseURL)
		if err != nil {
			return err
		}
		return compareRowCounts(counts, restored)
	}
	if err := restoreInto(ctx, engine, dir, partial, dump, check, io.Discard); err != nil {
		return err
	}
	if err := os.Rename(partial, dataDir); err != nil {
		return fmt.Errorf("move the new cluster into place: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	slog.Info("database moved to a new Postgres major", "data", dataDir, "kept", oldDataDir)
	return nil
}

// dumpOlder runs the old cluster with its own engine, counts its tables'
// rows, and dumps it with the new engine's pg_dump, which reads older
// servers, to a dump the new pg_restore reads.
func dumpOlder(ctx context.Context, oldEngine, engine, dir, oldDataDir, backups, from, to string) (dump string, counts map[string]int64, err error) {
	if backups == "" {
		return "", nil, errors.New("no backups folder to dump the database to first")
	}
	cluster, err := run(ctx, oldEngine, dir, oldDataDir, nil)
	if err != nil {
		return "", nil, fmt.Errorf("start the Postgres %s cluster: %w", from, err)
	}
	defer cluster.stopPostgres()
	if counts, err = countRows(ctx, cluster.URL()); err != nil {
		return "", nil, err
	}
	dump, err = databasebackup.DumpBeforeUpgrade(ctx, filepath.Join(engine, "bin", "pg_dump"), cluster.URL(), backups, from, to, time.Now())
	if err != nil {
		return "", nil, fmt.Errorf("dump the Postgres %s cluster: %w", from, err)
	}
	return dump, counts, nil
}

// startOlder runs the cluster of major from with its own engine, after the
// move to major to failed with upgradeErr, which it logs and the cluster
// keeps.
func startOlder(ctx context.Context, oldEngine, dir, from, to string, lock *os.File, upgradeErr error) (*Cluster, error) {
	fromMajor, _ := strconv.Atoi(from)
	toMajor, _ := strconv.Atoi(to)
	failure := &UpgradeError{From: fromMajor, To: toMajor, Err: upgradeErr}
	slog.Error("the database stays on its Postgres major; the next start tries the move again", "major", from, "error", failure)
	cluster, err := startCluster(ctx, oldEngine, dir, from, lock)
	if err != nil {
		return nil, fmt.Errorf("%w; and the Postgres %s cluster didn't start either: %w", failure, from, err)
	}
	cluster.upgradeErr = failure
	return cluster, nil
}

// refuseWithoutOldEngine is the error for a cluster of major from that the
// hub has no engine left to run, so no way to move it to major to: the owner
// restores the newest dump in backups instead.
func refuseWithoutOldEngine(dir, backups, from, to string) error {
	cause := fmt.Sprintf("the database in %s is Postgres %s, and this hub has no Postgres %s engine left to move it to Postgres %s with", filepath.Join(dir, from), from, from, to)
	newest, err := databasebackup.Newest(backups)
	if err != nil {
		return fmt.Errorf("%s, and the dumps in %s can't be read: %w", cause, backups, err)
	}
	if newest == "" {
		return fmt.Errorf("%s, nor a dump of it in %s: install a hub release that ships Postgres %s too, and start it once to move the database", cause, backups, from)
	}
	return fmt.Errorf("%s: restore the newest dump, %s, with hub-server database restore '%s', then start the hub", cause, filepath.Base(newest), newest)
}

// countRows counts the rows of every table in the database at databaseURL,
// by its schema-qualified name.
func countRows(ctx context.Context, databaseURL string) (map[string]int64, error) {
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	defer connection.Close(context.Background())
	rows, err := connection.Query(ctx, `
		SELECT n.nspname, c.relname
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r' AND n.nspname !~ '^pg_' AND n.nspname <> 'information_schema'`)
	if err != nil {
		return nil, fmt.Errorf("list the tables: %w", err)
	}
	tables, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pgx.Identifier, error) {
		var schema, name string
		err := row.Scan(&schema, &name)
		return pgx.Identifier{schema, name}, err
	})
	if err != nil {
		return nil, fmt.Errorf("list the tables: %w", err)
	}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := connection.QueryRow(ctx, "SELECT count(*) FROM "+table.Sanitize()).Scan(&count); err != nil {
			return nil, fmt.Errorf("count the rows of %s: %w", table.Sanitize(), err)
		}
		counts[table.Sanitize()] = count
	}
	return counts, nil
}

// compareRowCounts fails, naming each table, when a table is missing on
// either side or has another number of rows.
func compareRowCounts(before, after map[string]int64) error {
	var differences []string
	for table, count := range before {
		if restored, found := after[table]; !found {
			differences = append(differences, fmt.Sprintf("%s: %d rows before, missing after", table, count))
		} else if restored != count {
			differences = append(differences, fmt.Sprintf("%s: %d rows before, %d after", table, count, restored))
		}
	}
	for table, count := range after {
		if _, found := before[table]; !found {
			differences = append(differences, fmt.Sprintf("%s: missing before, %d rows after", table, count))
		}
	}
	if len(differences) == 0 {
		return nil
	}
	slices.Sort(differences)
	return fmt.Errorf("the tables differ: %s", strings.Join(differences, "; "))
}
