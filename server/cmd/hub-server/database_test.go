package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
)

// ownedDatabaseSettings is a server without HUB_DATABASE_URL whose engines
// folder holds the test's engine, and whose database folder is new.
func ownedDatabaseSettings(t *testing.T) (settings config, engine string) {
	t.Helper()
	engine = os.Getenv("HUB_TEST_POSTGRES_ENGINE")
	if engine == "" {
		t.Fatal("HUB_TEST_POSTGRES_ENGINE is not set; point it at a Postgres 18 installation's folder, the one holding bin/postgres, e.g. /usr/lib/postgresql/18")
	}
	engines := t.TempDir()
	if err := os.Symlink(engine, filepath.Join(engines, "postgres-18")); err != nil {
		t.Fatal(err)
	}
	// Short enough for the socket's path, which t.TempDir's can't promise.
	dir, err := os.MkdirTemp("", "hub-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	resolved, err := filepath.EvalSymlinks(engine)
	if err != nil {
		t.Fatal(err)
	}
	return config{postgresEngines: engines, postgresDir: dir}, resolved
}

func openOwnedDatabase(t *testing.T, settings config) *hubDatabase {
	t.Helper()
	database, err := openDatabase(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return database
}

func TestWithoutADatabaseURLTheServerRunsMigratesAndStopsItsOwnPostgres(t *testing.T) {
	settings, engine := ownedDatabaseSettings(t)
	ctx := context.Background()
	database := openOwnedDatabase(t, settings)

	// Migrated: the first migration's table is there.
	var companies int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM companies").Scan(&companies); err != nil {
		t.Fatalf("the database isn't migrated: %v", err)
	}
	if _, err := database.pool.Exec(ctx, "CREATE TABLE kept (note text); INSERT INTO kept VALUES ('still here')"); err != nil {
		t.Fatal(err)
	}

	health := httptest.NewServer(api.NewHealthHandler(database.pool))
	defer health.Close()
	response, err := http.Get(health.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("/v1/health answered %d", response.StatusCode)
	}

	// Nightly dumps use the engine's own pg_dump, over the socket.
	if want := filepath.Join(engine, "bin", "pg_dump"); database.pgDump != want {
		t.Fatalf("pg_dump = %s, want %s", database.pgDump, want)
	}
	if dump, err := databasebackup.NewDumper(database.url, t.TempDir(), database.pgDump).DumpIfDue(ctx); err != nil || dump == "" {
		t.Fatalf("dump %q, err %v", dump, err)
	}

	info, err := os.Stat(settings.postgresDir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the database folder: %v, %v", info, err)
	}

	pid := database.cluster.PID()
	dataDir := database.cluster.DataDir()
	database.Close()
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("postgres (PID %d) is still running after Close: %v", pid, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "postmaster.pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("postgres left its postmaster.pid: %v", err)
	}

	// The next start opens the same cluster, data and all.
	reopened := openOwnedDatabase(t, settings)
	var note string
	if err := reopened.pool.QueryRow(ctx, "SELECT note FROM kept").Scan(&note); err != nil || note != "still here" {
		t.Fatalf("after a restart: %q, %v", note, err)
	}
}

func TestASetPgDumpIsKeptWhenTheServerRunsItsOwnPostgres(t *testing.T) {
	settings, _ := ownedDatabaseSettings(t)
	settings.pgDump = "/opt/postgres/bin/pg_dump"
	if database := openOwnedDatabase(t, settings); database.pgDump != settings.pgDump {
		t.Fatalf("pg_dump = %s, want %s", database.pgDump, settings.pgDump)
	}
}

func TestNoEngineStopsTheStartBeforeAnyDatabase(t *testing.T) {
	dir := t.TempDir()
	_, err := openDatabase(context.Background(), config{postgresEngines: filepath.Join(dir, "engines"), postgresDir: filepath.Join(dir, "postgres")})
	if err == nil {
		t.Fatal("the database opened without an engine")
	}
	if _, err := os.Stat(filepath.Join(dir, "postgres")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the database folder was created: %v", err)
	}
}
