package databasebackup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func at(t *testing.T, moment string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", moment, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestADumpIsDueOncePerDayFromTheDueHour(t *testing.T) {
	for _, test := range []struct {
		name string
		now  string
		days []string
		want bool
	}{
		{"no dump yet, even at night", "2026-09-30 01:00", nil, true},
		{"yesterday's, before the due hour", "2026-09-30 02:59", []string{"2026-09-29"}, false},
		{"yesterday's, from the due hour", "2026-09-30 03:00", []string{"2026-09-29"}, true},
		{"an older one, later in the day", "2026-09-30 18:00", []string{"2026-09-20", "2026-09-27"}, true},
		{"today's already", "2026-09-30 23:00", []string{"2026-09-29", "2026-09-30"}, false},
	} {
		if got := isDumpDue(at(t, test.now), test.days); got != test.want {
			t.Errorf("%s: due = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestOnlyTheNewestDumpsAreKept(t *testing.T) {
	folder := t.TempDir()
	start := at(t, "2026-09-01 03:00")
	for day := range 16 {
		name := dumpPrefix + start.AddDate(0, 0, day).Format(dumpDateLayout) + dumpSuffix
		if err := os.WriteFile(filepath.Join(folder, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(folder, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	dumper := NewDumper("", folder, "")
	if err := dumper.removeOldDumps(); err != nil {
		t.Fatal(err)
	}
	days, err := dumper.listDumpDays()
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != keptDumps || days[0] != "2026-09-03" || days[len(days)-1] != "2026-09-16" {
		t.Fatalf("kept %v, want 2026-09-03 through 2026-09-16", days)
	}
	if _, err := os.Stat(filepath.Join(folder, "notes.txt")); err != nil {
		t.Errorf("another file in the folder was touched: %v", err)
	}
}

func TestOnlyTheNewestPreMigrationDumpsAreKept(t *testing.T) {
	folder := t.TempDir()
	written := at(t, "2026-09-01 03:00")
	// Version 70 is written last, as after a restore of an older dump.
	for order, version := range []string{"77", "78", "79", "80", "81", "82", "70"} {
		path := filepath.Join(folder, preMigrationPrefix+version+dumpSuffix)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := written.Add(time.Duration(order) * time.Hour)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	nightly := dumpPrefix + "2026-08-01" + dumpSuffix
	for _, name := range []string{nightly, "notes.txt"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeOldPreMigrationDumps(folder); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	want := []string{"hub-2026-08-01.dump", "hub-pre-migration-70.dump", "hub-pre-migration-79.dump", "hub-pre-migration-80.dump", "hub-pre-migration-81.dump", "hub-pre-migration-82.dump", "notes.txt"}
	if !slices.Equal(names, want) {
		t.Fatalf("kept %v, want %v", names, want)
	}

	// The nightly listing leaves the pre-migration dumps out.
	days, err := NewDumper("", folder, "").listDumpDays()
	if err != nil || !slices.Equal(days, []string{"2026-08-01"}) {
		t.Fatalf("nightly dumps = %v, %v", days, err)
	}
}

func TestThePasswordStaysOffTheCommandLine(t *testing.T) {
	command, err := buildDumpCommand(context.Background(), "pg_dump", "postgres://hub:s3cret@localhost:5434/hub", "out.dump")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(command.Args, " "), "s3cret") {
		t.Fatalf("the password is in the arguments: %v", command.Args)
	}
	if !slices.Contains(command.Args, "--dbname=postgres://hub@localhost:5434/hub") {
		t.Errorf("args = %v, want the URL without its password", command.Args)
	}
	if !slices.Contains(command.Env, "PGPASSWORD=s3cret") {
		t.Error("PGPASSWORD is not set for pg_dump")
	}
}

func TestADumpIsReadableAndAFailedOneLeavesNoFile(t *testing.T) {
	pgDump, err := exec.LookPath("pg_dump")
	if err != nil {
		t.Skip("pg_dump is not installed")
	}
	pool := testdatabase.New(t)
	folder := t.TempDir()
	now := at(t, "2026-09-30 03:10")

	dumper := NewDumper(pool.Config().ConnString(), folder, pgDump)
	dumper.now = func() time.Time { return now }
	path, err := dumper.DumpIfDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "hub-2026-09-30.dump" {
		t.Fatalf("dumped to %s", path)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("dump file: %v, %v", info, err)
	}
	if output, err := exec.Command("pg_restore", "--list", path).CombinedOutput(); err != nil {
		t.Fatalf("pg_restore can't read the dump: %v: %s", err, output)
	}
	if again, err := dumper.DumpIfDue(context.Background()); err != nil || again != "" {
		t.Fatalf("a second dump the same day: %q, %v", again, err)
	}

	broken := NewDumper("postgres://nobody@localhost:1/nothing", t.TempDir(), pgDump)
	if _, err := broken.Dump(context.Background(), now); err == nil {
		t.Fatal("expected an error from an unreachable database")
	}
	if entries, _ := os.ReadDir(broken.folder); len(entries) != 0 {
		t.Errorf("a failed dump left %d files", len(entries))
	}
}
