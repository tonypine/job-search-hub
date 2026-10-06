package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

func readNote(t *testing.T, database *hubDatabase) string {
	t.Helper()
	var note string
	if err := database.pool.QueryRow(context.Background(), "SELECT note FROM kept").Scan(&note); err != nil {
		t.Fatal(err)
	}
	return note
}

func TestTheRestoreCommandIsRefusedWhileTheServerRunsThenBringsADumpsRowsBack(t *testing.T) {
	settings, _ := ownedDatabaseSettings(t)
	database := openOwnedDatabase(t, settings)
	execute(t, database.url, "CREATE TABLE kept (note text); INSERT INTO kept VALUES ('from the dump')")
	dump, err := databasebackup.NewDumper(database.url, t.TempDir(), database.pgDump).Dump(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	execute(t, database.url, "UPDATE kept SET note = 'after the dump'")
	lookup := lookupFrom(map[string]string{"HUB_POSTGRES_ENGINES": settings.postgresEngines, "HUB_POSTGRES_DIR": settings.postgresDir})

	var out bytes.Buffer
	err = runDatabaseCommand([]string{"restore", dump}, lookup, &out)
	if err == nil || !strings.Contains(err.Error(), "stop the hub in Settings › Server first") {
		t.Fatalf("a restore while the server runs: %v", err)
	}
	if note := readNote(t, database); note != "after the dump" {
		t.Fatalf("the refused restore changed the database: %q", note)
	}

	database.Close()
	out.Reset()
	if err := runDatabaseCommand([]string{"restore", dump}, lookup, &out); err != nil {
		t.Fatalf("restore: %v\n%s", err, out.String())
	}
	for _, want := range []string{"pg_restore: ", ".replaced-", "delete that folder once the hub runs well"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the output doesn't say %q:\n%s", want, out.String())
		}
	}
	if note := readNote(t, openOwnedDatabase(t, settings)); note != "from the dump" {
		t.Fatalf("after the restore: %q", note)
	}
}

func TestTheRestoreCommandRefusesADumpThatHoldsNoHubDatabase(t *testing.T) {
	ctx := context.Background()
	settings, engine := ownedDatabaseSettings(t)
	database := openOwnedDatabase(t, settings)
	execute(t, database.url, "CREATE TABLE kept (note text); INSERT INTO kept VALUES ('current')")
	database.Close()

	// A dump of a database the hub never migrated.
	other, _ := ownedDatabaseSettings(t)
	cluster, err := postgresprocess.Start(ctx, postgresprocess.Settings{Engine: engine, Dir: other.postgresDir})
	if err != nil {
		t.Fatal(err)
	}
	execute(t, cluster.URL(), "CREATE TABLE elsewhere (note text)")
	dump, err := databasebackup.NewDumper(cluster.URL(), t.TempDir(), filepath.Join(engine, "bin", "pg_dump")).Dump(ctx, time.Now())
	cluster.Stop()
	if err != nil {
		t.Fatal(err)
	}

	err = restoreDatabase(ctx, settings.postgresEngines, settings.postgresDir, dump, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "holds no hub database") || !strings.Contains(err.Error(), "as it was") {
		t.Fatalf("a dump that isn't the hub's: %v", err)
	}
	if note := readNote(t, openOwnedDatabase(t, settings)); note != "current" {
		t.Fatalf("the current database changed: %q", note)
	}
}

func TestTheDatabaseCommandNeedsRestoreAndOneDump(t *testing.T) {
	for _, arguments := range [][]string{nil, {"restore"}, {"dump", "hub.dump"}, {"restore", "a.dump", "b.dump"}} {
		err := runDatabaseCommand(arguments, lookupFrom(nil), &bytes.Buffer{})
		if err == nil || err.Error() != databaseUsage {
			t.Errorf("%v: %v, want the usage", arguments, err)
		}
	}
}
