package databasebackup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/drain"
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

func TestAPreUpgradeDumpIsNamedByBothMajorsAndOutlivesThePruning(t *testing.T) {
	folder := t.TempDir()
	// A pg_dump that writes its --file argument and nothing else.
	pgDump := filepath.Join(t.TempDir(), "pg_dump")
	script := "#!/bin/sh\nfor argument; do case $argument in --file=*) echo dump > \"${argument#--file=}\";; esac; done\n"
	if err := os.WriteFile(pgDump, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := DumpBeforeUpgrade(context.Background(), pgDump, "postgres:///hub", folder, "17", "18", at(t, "2026-10-06 09:30"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(folder, "hub-pre-upgrade-17-to-18-2026-10-06.dump"); path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the dump: %v, %v", info, err)
	}

	// Neither the nightly nor the pre-migration pruning counts it.
	if days, err := NewDumper("", folder, "").listDumpDays(); err != nil || len(days) != 0 {
		t.Fatalf("nightly dumps = %v, %v", days, err)
	}
	if err := removeOldPreMigrationDumps(folder); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the pruning removed it: %v", err)
	}
}

func TestTheNewestDumpIsTheOneWrittenLastOfAnyKind(t *testing.T) {
	folder := t.TempDir()
	if newest, err := Newest(filepath.Join(folder, "missing")); err != nil || newest != "" {
		t.Fatalf("no folder: %q, %v", newest, err)
	}
	if newest, err := Newest(folder); err != nil || newest != "" {
		t.Fatalf("an empty folder: %q, %v", newest, err)
	}

	written := at(t, "2026-09-01 03:00")
	// A later name isn't a later dump: the pre-migration one is written last.
	for order, name := range []string{"hub-2026-09-30.dump", "hub-pre-upgrade-17-to-18-2026-09-29.dump", "hub-pre-migration-80.dump", "hub-2026-10-01.dump.partial", "notes.txt"} {
		path := filepath.Join(folder, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := written.Add(time.Duration(order) * time.Hour)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	newest, err := Newest(folder)
	if want := filepath.Join(folder, "hub-pre-migration-80.dump"); err != nil || newest != want {
		t.Fatalf("newest = %q, %v; want %s", newest, err, want)
	}
}

func TestThePasswordStaysOffTheCommandLine(t *testing.T) {
	command, err := buildDumpCommand(context.Background(), "pg_dump", "postgres://hub:s3cret@db.example:5432/hub", "out.dump")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(command.Args, " "), "s3cret") {
		t.Fatalf("the password is in the arguments: %v", command.Args)
	}
	if !slices.Contains(command.Args, "--dbname=postgres://hub@db.example:5432/hub") {
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

func TestAnImportsDumpIsNeverCountedOrRemoved(t *testing.T) {
	folder := t.TempDir()
	imported := filepath.Join(folder, importPrefix+"2026-08-01"+dumpSuffix)
	if err := os.WriteFile(imported, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	start := at(t, "2026-09-01 03:00")
	for day := range keptDumps + 2 {
		name := dumpPrefix + start.AddDate(0, 0, day).Format(dumpDateLayout) + dumpSuffix
		if err := os.WriteFile(filepath.Join(folder, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for version := range keptPreMigrationDumps + 2 {
		name := preMigrationPrefix + strconv.Itoa(70+version) + dumpSuffix
		if err := os.WriteFile(filepath.Join(folder, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	dumper := NewDumper("", folder, "")
	if err := dumper.removeOldDumps(); err != nil {
		t.Fatal(err)
	}
	if err := removeOldPreMigrationDumps(folder); err != nil {
		t.Fatal(err)
	}
	days, err := dumper.listDumpDays()
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != keptDumps || slices.Contains(days, "2026-08-01") {
		t.Fatalf("nightly dumps %v", days)
	}
	if _, err := os.Stat(imported); err != nil {
		t.Fatalf("the import's dump was removed: %v", err)
	}
}

func TestNoPassStartsWhileTheHubDrains(t *testing.T) {
	folder := t.TempDir()
	ran := filepath.Join(folder, "ran")
	pgDump := filepath.Join(folder, "pg_dump")
	if err := os.WriteFile(pgDump, []byte("#!/bin/sh\ntouch "+ran+"\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dumper := NewDumper("postgres://nobody@localhost:1/nothing", t.TempDir(), pgDump)
	dumper.now = func() time.Time { return at(t, "2026-09-30 03:10") }
	hub := drain.New(time.Hour)
	hub.Start()
	ctx, cancel := context.WithCancel(drain.NewContext(context.Background(), hub))
	stopped := make(chan struct{})
	go func() {
		dumper.Run(ctx, 10*time.Millisecond)
		close(stopped)
	}()
	defer func() {
		cancel()
		<-stopped
	}()

	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("a backup pass ran while the hub drained")
	}
	hub.Cancel()
	for range 200 {
		if _, err := os.Stat(ran); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no pass ran once the drain was cancelled")
}
