package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/migrations"
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

	health := httptest.NewServer(api.NewHealthHandler(database.pool, database.postgresHealth()))
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

func execute(t *testing.T, databaseURL, statement string) {
	t.Helper()
	connection, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	if _, err := connection.Exec(context.Background(), statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func listFolder(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// migrateAllButTheLast migrates the database at databaseURL up to the
// migration before the newest, as a database an app update finds, and
// returns that migration's version.
func migrateAllButTheLast(t *testing.T, databaseURL string) int64 {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	database := stdlib.OpenDBFromPool(pool)
	defer database.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	sources := provider.ListSources()
	version := sources[len(sources)-2].Version
	if _, err := provider.UpTo(ctx, version); err != nil {
		t.Fatal(err)
	}
	return version
}

func TestPendingMigrationsAreDumpedFirstAndAStartWithNonePendingTakesNoDump(t *testing.T) {
	ctx := context.Background()

	// A new database has nothing to keep.
	fresh, _ := ownedDatabaseSettings(t)
	fresh.backupsDir = t.TempDir()
	openOwnedDatabase(t, fresh).Close()
	if names := listFolder(t, fresh.backupsDir); len(names) != 0 {
		t.Fatalf("a new database was dumped: %v", names)
	}

	// One a migration behind, as an app update finds it.
	settings, engine := ownedDatabaseSettings(t)
	settings.backupsDir = t.TempDir()
	cluster, err := postgresprocess.Start(ctx, postgresprocess.Settings{Engine: engine, Dir: settings.postgresDir})
	if err != nil {
		t.Fatal(err)
	}
	version := migrateAllButTheLast(t, cluster.URL())
	execute(t, cluster.URL(), "CREATE TABLE kept (note text); INSERT INTO kept VALUES ('before the migration')")
	cluster.Stop()

	openOwnedDatabase(t, settings).Close()
	dump := fmt.Sprintf("hub-pre-migration-%d.dump", version)
	if names := listFolder(t, settings.backupsDir); len(names) != 1 || names[0] != dump {
		t.Fatalf("backups = %v, want [%s]", names, dump)
	}
	output, err := exec.Command(filepath.Join(engine, "bin", "pg_restore"), "--list", filepath.Join(settings.backupsDir, dump)).CombinedOutput()
	if err != nil {
		t.Fatalf("pg_restore can't read the dump: %v: %s", err, output)
	}
	if !strings.Contains(string(output), "TABLE public kept") {
		t.Fatalf("the dump doesn't hold the database's tables:\n%s", output)
	}

	// Migrated now: the next start has nothing to dump.
	openOwnedDatabase(t, settings).Close()
	if names := listFolder(t, settings.backupsDir); len(names) != 1 {
		t.Fatalf("a start with nothing pending dumped: %v", names)
	}
}
