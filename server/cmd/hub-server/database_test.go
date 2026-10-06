package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
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

// externalDatabaseURL is a new, migrated database's URL, like a
// HUB_DATABASE_URL naming Docker's Postgres.
func externalDatabaseURL(t *testing.T) string {
	t.Helper()
	var name string
	if err := testdatabase.New(t).QueryRow(context.Background(), "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	address, err := url.Parse(os.Getenv("HUB_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	address.Path = "/" + name
	return address.String()
}

func TestADatabaseURLKeepsTheServerOnThatPostgres(t *testing.T) {
	databaseURL := externalDatabaseURL(t)
	ctx := context.Background()
	database := openOwnedDatabase(t, config{databaseURL: databaseURL})

	var companies int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM companies").Scan(&companies); err != nil {
		t.Fatalf("the database isn't migrated: %v", err)
	}
	if database.url != databaseURL {
		t.Fatalf("url = %s, want %s", database.url, databaseURL)
	}
	// Dumps use the pg_dump on the PATH, as before the server ran Postgres.
	if database.pgDump != "pg_dump" {
		t.Fatalf("pg_dump = %s, want pg_dump", database.pgDump)
	}
	if database.cluster != nil {
		t.Fatal("the server started a Postgres of its own")
	}
	if database.Exited() != nil || database.ExitError() != nil {
		t.Fatalf("a Postgres the server doesn't run exited: %v", database.ExitError())
	}

	pgDump := "/usr/lib/postgresql/18/bin/pg_dump"
	if kept := openOwnedDatabase(t, config{databaseURL: databaseURL, pgDump: pgDump}); kept.pgDump != pgDump {
		t.Fatalf("pg_dump = %s, want %s", kept.pgDump, pgDump)
	}
}

func TestTheServersPostgresStoppingByItselfIsSignalled(t *testing.T) {
	settings, _ := ownedDatabaseSettings(t)
	database := openOwnedDatabase(t, settings)
	if err := syscall.Kill(database.cluster.PID(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	select {
	case <-database.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("Exited didn't close after postgres was killed")
	}
	if database.ExitError() == nil {
		t.Fatal("ExitError is nil after postgres was killed")
	}

	closed := make(chan struct{})
	go func() {
		database.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close hung after postgres was killed")
	}
}
