package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

func TestCountWritesSaysWhatTheServerWroteAfterMigrating(t *testing.T) {
	ctx := context.Background()
	settings, engine := ownedDatabaseSettings(t)
	settings.backupsDir = t.TempDir()
	cluster, err := postgresprocess.Start(ctx, postgresprocess.Settings{Engine: engine, Dir: settings.postgresDir})
	if err != nil {
		t.Fatal(err)
	}
	version := strconv.FormatInt(migrateAllButTheLast(t, cluster.URL()), 10)
	cluster.Stop()

	count := func() writesSinceMigration {
		t.Helper()
		var out bytes.Buffer
		if err := countWritesSinceMigration(ctx, settings.postgresEngines, settings.postgresDir, settings.backupsDir, version, &out); err != nil {
			t.Fatal(err)
		}
		var result writesSinceMigration
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("%v: %s", err, out.String())
		}
		return result
	}

	// Before any install, there's no dump to go back to.
	if result := count(); result.Dump != "" || result.Marked {
		t.Fatalf("before migrating: %+v", result)
	}

	// The new version dumps, migrates and marks; its stop keeps the
	// counters, as a clean stop of Postgres does.
	database := openOwnedDatabase(t, settings)
	// While the server holds the database, the count waits for its stop.
	if err := countWritesSinceMigration(ctx, settings.postgresEngines, settings.postgresDir, settings.backupsDir, version, &bytes.Buffer{}); !errors.Is(err, postgresprocess.ErrLocked) {
		t.Fatalf("a count while the server runs: %v, want it refused", err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO updates (kind, title) VALUES ('reply', 'Acme replied'), ('reply', 'Initech replied')`); err != nil {
		t.Fatal(err)
	}
	database.Close()

	result := count()
	if !result.Marked || result.Count == nil || result.Dump == "" || result.DumpedAt == nil {
		t.Fatalf("after the install: %+v", result)
	}
	if result.Lost != "Lost: 2 updates." {
		t.Fatalf("lost = %q, want the two updates", result.Lost)
	}
}
