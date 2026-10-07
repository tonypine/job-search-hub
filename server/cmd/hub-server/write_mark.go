package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// writeMark writes every table's write counters to path, beside the dump
// taken before migrating, through a file renamed into place.
func writeMark(ctx context.Context, pool *pgxpool.Pool, path string) error {
	mark, err := store.ReadWriteMark(ctx, pool)
	if err != nil {
		return fmt.Errorf("mark the database's writes after migrating: %w", err)
	}
	data, err := json.MarshalIndent(mark, "", "  ")
	if err != nil {
		return err
	}
	partial := path + ".partial"
	if err := os.WriteFile(partial, data, 0o600); err != nil {
		return fmt.Errorf("write the mark of the database's writes: %w", err)
	}
	if err := os.Rename(partial, path); err != nil {
		return fmt.Errorf("write the mark of the database's writes: %w", err)
	}
	return nil
}

// writesSinceMigration is what hub-server database count-writes prints:
// the dump taken before migrating from a version, and what was written
// since the migrations, which restoring the dump loses.
type writesSinceMigration struct {
	// Dump is the dump's path; empty when there is none.
	Dump     string     `json:"dump,omitempty"`
	DumpedAt *time.Time `json:"dumped_at,omitempty"`
	// Marked says the migrations finished and the server took its mark,
	// after which it could serve writes. Without a mark it served none.
	Marked bool `json:"marked"`
	// Count is what was written since the mark; nil without one.
	Count *store.WriteCount `json:"count,omitempty"`
	// Lost says it in the owner's words: "Nothing was lost." and so on.
	Lost string `json:"lost,omitempty"`
}

// countWritesSinceMigration finds the dump taken before migrating from
// version in backups and its mark, starts the database the server owns to
// read the counters, and prints what was written since as JSON. It runs
// while the server is stopped, whose clean stop kept the counters.
func countWritesSinceMigration(ctx context.Context, engines, dir, backups, version string, out io.Writer) error {
	if _, err := strconv.ParseInt(version, 10, 64); err != nil {
		return fmt.Errorf("%q isn't a migration version", version)
	}
	result := writesSinceMigration{}
	dump := filepath.Join(backups, "hub-pre-migration-"+version+".dump")
	info, err := os.Stat(dump)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return json.NewEncoder(out).Encode(result)
	case err != nil:
		return err
	}
	dumpedAt := info.ModTime()
	result.Dump, result.DumpedAt = dump, &dumpedAt

	data, err := os.ReadFile(databasebackup.MarkPath(dump))
	if errors.Is(err, os.ErrNotExist) {
		result.Lost = "Nothing was lost."
		return json.NewEncoder(out).Encode(result)
	}
	if err != nil {
		return err
	}
	var mark store.WriteMark
	if err := json.Unmarshal(data, &mark); err != nil {
		return fmt.Errorf("read the mark beside %s: %w", dump, err)
	}
	result.Marked = true

	engine, err := postgresprocess.NewestEngine(engines)
	if err != nil {
		return err
	}
	cluster, err := postgresprocess.Start(ctx, postgresprocess.Settings{Engine: engine, Dir: dir})
	if errors.Is(err, postgresprocess.ErrLocked) {
		return fmt.Errorf("%w; stop the hub in Settings › Server first", err)
	}
	if err != nil {
		return fmt.Errorf("start the database: %w", err)
	}
	defer cluster.Stop()
	pool, err := pgxpool.New(ctx, cluster.URL())
	if err != nil {
		return err
	}
	defer pool.Close()
	count, err := store.CountWrites(ctx, pool, mark)
	if err != nil {
		return err
	}
	result.Count = &count
	result.Lost = count.Describe(time.Local)
	return json.NewEncoder(out).Encode(result)
}
