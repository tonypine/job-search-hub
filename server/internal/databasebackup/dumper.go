// Package databasebackup dumps the hub's database into a private folder on
// this machine: once a night, and before migrations or an engine upgrade
// change it. It keeps the newest nightly and pre-migration dumps. It also
// dumps the database an import moves the hub's data from, and keeps that dump.
package databasebackup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/drain"
)

const (
	// keptDumps is how many nightly dumps stay; older ones are removed.
	keptDumps = 14
	// keptPreMigrationDumps is how many dumps taken before migrations stay.
	keptPreMigrationDumps = 5
	// dueHour is the local hour from which the day's dump is due.
	dueHour = 3

	dumpPrefix     = "hub-"
	dumpSuffix     = ".dump"
	dumpDateLayout = "2006-01-02"
	// A dump taken before migrations is named by the migration version it
	// holds. The nightly dumps' listing leaves it out: its name holds no day.
	preMigrationPrefix = "hub-pre-migration-"
	// A dump taken before moving the database to a new Postgres major is
	// kept until it is removed by hand.
	preUpgradePrefix = "hub-pre-upgrade-"
	// A dump of the database an import moves the hub's data from, named by
	// its day. Neither listing counts it, so none removes it.
	importPrefix = "hub-import-"
)

// Dumper writes one pg_dump a day of the database at databaseURL into
// folder, named by its day, with the pg_dump binary at pgDump.
type Dumper struct {
	databaseURL string
	folder      string
	pgDump      string
	now         func() time.Time
}

func NewDumper(databaseURL, folder, pgDump string) *Dumper {
	return &Dumper{databaseURL: databaseURL, folder: folder, pgDump: pgDump, now: time.Now}
}

// Run dumps the database whenever a dump is due, checking every interval.
func (dumper *Dumper) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			if path, err := dumper.DumpIfDue(ctx); err != nil {
				slog.Error("database backup failed", "error", err)
			} else if path != "" {
				slog.Info("database backed up", "file", path)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// DumpIfDue dumps the database when there is no dump yet, or when the
// newest is from an earlier day and it is past dueHour. It returns the new
// dump's path, or "" when none was due.
func (dumper *Dumper) DumpIfDue(ctx context.Context) (string, error) {
	days, err := dumper.listDumpDays()
	if err != nil {
		return "", err
	}
	now := dumper.now()
	if !isDumpDue(now, days) {
		return "", nil
	}
	return dumper.Dump(ctx, now)
}

// isDumpDue reports whether a dump is due at now, given the days of the
// dumps already kept, oldest first.
func isDumpDue(now time.Time, days []string) bool {
	if len(days) == 0 {
		return true
	}
	return days[len(days)-1] < now.Format(dumpDateLayout) && now.Hour() >= dueHour
}

// Dump writes the database to the folder as the dump of now's day, then
// removes the dumps beyond keptDumps. A failed dump leaves no file behind.
func (dumper *Dumper) Dump(ctx context.Context, now time.Time) (string, error) {
	path := filepath.Join(dumper.folder, dumpPrefix+now.Format(dumpDateLayout)+dumpSuffix)
	if err := writeDump(ctx, dumper.pgDump, dumper.databaseURL, path); err != nil {
		return "", err
	}
	return path, dumper.removeOldDumps()
}

// DumpBeforeMigration writes the database at databaseURL, migrated up to
// version, to folder as hub-pre-migration-<version>.dump with the pg_dump at
// pgDump, so a migration that goes wrong can be undone. Then it removes all
// but the newest keptPreMigrationDumps of them.
func DumpBeforeMigration(ctx context.Context, pgDump, databaseURL, folder string, version int64) (string, error) {
	path := filepath.Join(folder, preMigrationPrefix+strconv.FormatInt(version, 10)+dumpSuffix)
	if err := writeDump(ctx, pgDump, databaseURL, path); err != nil {
		return "", err
	}
	return path, removeOldPreMigrationDumps(folder)
}

// DumpBeforeUpgrade writes the database at databaseURL, a cluster of Postgres
// from, to folder as hub-pre-upgrade-<from>-to-<to>-<date>.dump with the
// pg_dump at pgDump, a newer one's, before the database moves to Postgres to.
func DumpBeforeUpgrade(ctx context.Context, pgDump, databaseURL, folder, from, to string, now time.Time) (string, error) {
	path := filepath.Join(folder, preUpgradePrefix+from+"-to-"+to+"-"+now.Format(dumpDateLayout)+dumpSuffix)
	if err := writeDump(ctx, pgDump, databaseURL, path); err != nil {
		return "", err
	}
	return path, nil
}

// Newest is the dump in folder written last, of any kind, or "" when there
// is none.
func Newest(folder string) (string, error) {
	entries, err := os.ReadDir(folder)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	newest, newestModified := "", time.Time{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), dumpPrefix) || !strings.HasSuffix(entry.Name(), dumpSuffix) || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		if newest == "" || info.ModTime().After(newestModified) {
			newest, newestModified = filepath.Join(folder, entry.Name()), info.ModTime()
		}
	}
	return newest, nil
}

// DumpForImport writes the database at databaseURL, which an import moves
// the hub's data from, to folder as hub-import-<now's day>.dump with the
// pg_dump at pgDump. It is kept until the owner removes it.
func DumpForImport(ctx context.Context, pgDump, databaseURL, folder string, now time.Time) (string, error) {
	path := filepath.Join(folder, importPrefix+now.Format(dumpDateLayout)+dumpSuffix)
	if err := writeDump(ctx, pgDump, databaseURL, path); err != nil {
		return "", err
	}
	return path, nil
}

// writeDump dumps the database to path, through a partial file renamed into
// place once pg_dump succeeds, so a failed dump leaves no file behind.
func writeDump(ctx context.Context, pgDump, databaseURL, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create the backups folder: %w", err)
	}
	partial := path + ".partial"
	command, err := buildDumpCommand(ctx, pgDump, databaseURL, partial)
	if err != nil {
		return err
	}
	if output, err := command.CombinedOutput(); err != nil {
		_ = os.Remove(partial)
		return fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Chmod(partial, 0o600); err != nil {
		return err
	}
	return os.Rename(partial, path)
}

// buildDumpCommand runs pg_dump in its custom format into file. The password
// goes through PGPASSWORD rather than the command line, where any process
// on the machine could read it.
func buildDumpCommand(ctx context.Context, pgDump, databaseURL, file string) (*exec.Cmd, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return nil, errors.New("the database URL doesn't parse")
	}
	environment := os.Environ()
	if parsed.User != nil {
		if password, has := parsed.User.Password(); has {
			environment = append(environment, "PGPASSWORD="+password)
			parsed.User = url.User(parsed.User.Username())
		}
	}
	command := exec.CommandContext(ctx, pgDump, "--format=custom", "--file="+file, "--dbname="+parsed.String())
	command.Env = environment
	return command, nil
}

// listDumpDays returns the days of the dumps in the folder, oldest first.
// Other files in the folder are left out.
func (dumper *Dumper) listDumpDays() ([]string, error) {
	entries, err := os.ReadDir(dumper.folder)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var days []string
	for _, entry := range entries {
		day, isDump := strings.CutPrefix(entry.Name(), dumpPrefix)
		day, hasSuffix := strings.CutSuffix(day, dumpSuffix)
		if !isDump || !hasSuffix {
			continue
		}
		if _, err := time.Parse(dumpDateLayout, day); err == nil {
			days = append(days, day)
		}
	}
	slices.Sort(days)
	return days, nil
}

func (dumper *Dumper) removeOldDumps() error {
	days, err := dumper.listDumpDays()
	if err != nil || len(days) <= keptDumps {
		return err
	}
	for _, day := range days[:len(days)-keptDumps] {
		if err := os.Remove(filepath.Join(dumper.folder, dumpPrefix+day+dumpSuffix)); err != nil {
			return err
		}
	}
	return nil
}

// removeOldPreMigrationDumps keeps the keptPreMigrationDumps written last. A
// dump of an older version can be the newest, after a restore.
func removeOldPreMigrationDumps(folder string) error {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return err
	}
	type dump struct {
		name     string
		modified time.Time
	}
	var dumps []dump
	for _, entry := range entries {
		version, isDump := strings.CutPrefix(entry.Name(), preMigrationPrefix)
		version, hasSuffix := strings.CutSuffix(version, dumpSuffix)
		if !isDump || !hasSuffix || !entry.Type().IsRegular() {
			continue
		}
		if _, err := strconv.ParseInt(version, 10, 64); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		dumps = append(dumps, dump{entry.Name(), info.ModTime()})
	}
	if len(dumps) <= keptPreMigrationDumps {
		return nil
	}
	slices.SortFunc(dumps, func(a, b dump) int { return a.modified.Compare(b.modified) })
	for _, old := range dumps[:len(dumps)-keptPreMigrationDumps] {
		if err := os.Remove(filepath.Join(folder, old.name)); err != nil {
			return err
		}
	}
	return nil
}
