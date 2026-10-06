package postgresprocess_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

// The majors of HUB_TEST_POSTGRES_OLD_ENGINE and HUB_TEST_POSTGRES_ENGINE.
const (
	oldMajor = "17"
	newMajor = "18"
)

func getOldEngine(t *testing.T) string {
	t.Helper()
	engine := os.Getenv("HUB_TEST_POSTGRES_OLD_ENGINE")
	if engine == "" {
		t.Fatal("HUB_TEST_POSTGRES_OLD_ENGINE is not set; point it at a Postgres 17 installation's folder, the one holding bin/postgres, e.g. /usr/lib/postgresql/17")
	}
	return engine
}

// newEngines is an engines folder holding the given engines by major, as an
// app update that brings a new major leaves it.
func newEngines(t *testing.T, engines map[string]string) string {
	t.Helper()
	folder := t.TempDir()
	for major, engine := range engines {
		if err := os.Symlink(engine, filepath.Join(folder, "postgres-"+major)); err != nil {
			t.Fatal(err)
		}
	}
	return folder
}

// bothEngines is an engines folder with the old engine and the new one.
func bothEngines(t *testing.T) string {
	t.Helper()
	return newEngines(t, map[string]string{oldMajor: getOldEngine(t), newMajor: getEngine(t)})
}

// oldCluster is a stopped cluster of the old major in a new folder, holding
// what statements put in it.
func oldCluster(t *testing.T, statements string) (dir string) {
	t.Helper()
	dir = newDir(t)
	cluster, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getOldEngine(t), Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	connection := connect(t, cluster)
	execute(t, connection, statements)
	connection.Close(context.Background())
	cluster.Stop()
	if cluster.Major() != 17 {
		t.Fatalf("the old engine is Postgres %d, want %s", cluster.Major(), oldMajor)
	}
	return dir
}

// startUpgrading starts the new engine's cluster in dir, with engines and
// backups for the move from an older major.
func startUpgrading(t *testing.T, dir, engines, backups string) (*postgresprocess.Cluster, error) {
	t.Helper()
	cluster, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir, Engines: engines, Backups: backups})
	if err == nil {
		t.Cleanup(cluster.Stop)
	}
	return cluster, err
}

func listNames(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

const hubLikeTables = `
CREATE TABLE companies (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	name text NOT NULL,
	facts jsonb NOT NULL,
	tags text[] NOT NULL
);
INSERT INTO companies (name, facts, tags)
	SELECT 'Company ' || n,
		jsonb_build_object('n', n, 'remote', n % 2 = 0, 'offices', jsonb_build_array('Lisbon', 'São Paulo')),
		ARRAY['tag' || n, CASE WHEN n % 5 = 0 THEN 'go' ELSE 'swift' END]
	FROM generate_series(1, 50) AS n;
CREATE TABLE notes (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	company_id uuid NOT NULL REFERENCES companies,
	body text NOT NULL,
	scores integer[]
);
INSERT INTO notes (company_id, body, scores)
	SELECT id, 'note ' || n, ARRAY[n, n * 2] FROM companies, generate_series(1, 3) AS n;
CREATE SCHEMA archive;
CREATE TABLE archive.events (at timestamptz NOT NULL DEFAULT now(), payload jsonb);
INSERT INTO archive.events (payload) SELECT jsonb_build_object('event', n) FROM generate_series(1, 7) AS n;
CREATE TABLE "Empty Table" (note text);
`

func TestAnOlderMajorsClusterMovesToTheEnginesMajorWithEveryRowAndIsKept(t *testing.T) {
	dir := oldCluster(t, hubLikeTables)
	backups := t.TempDir()

	cluster, err := startUpgrading(t, dir, bothEngines(t), backups)
	if err != nil {
		t.Fatal(err)
	}
	if failure := cluster.UpgradeError(); failure != nil {
		t.Fatalf("the upgrade failed: %v", failure)
	}
	if cluster.Major() != 18 || cluster.DataDir() != filepath.Join(dir, newMajor) {
		t.Fatalf("running Postgres %d in %s, want %s in %s", cluster.Major(), cluster.DataDir(), newMajor, filepath.Join(dir, newMajor))
	}
	connection := connect(t, cluster)
	if version := queryString(t, connection, "SELECT (current_setting('server_version_num')::int / 10000)::text"); version != newMajor {
		t.Fatalf("the server is Postgres %s", version)
	}
	for query, want := range map[string]string{
		"SELECT count(*)::text FROM companies":                                 "50",
		"SELECT count(*)::text FROM notes":                                     "150",
		"SELECT count(*)::text FROM archive.events":                            "7",
		`SELECT count(*)::text FROM "Empty Table"`:                             "0",
		"SELECT count(*)::text FROM companies WHERE facts->>'remote' = 'true'": "25",
		"SELECT facts->'offices'->>1 FROM companies WHERE name = 'Company 1'":  "São Paulo",
		"SELECT count(*)::text FROM companies WHERE 'go' = ANY(tags)":          "10",
		"SELECT sum(scores[2])::text FROM notes":                               "600",
	} {
		if got := queryString(t, connection, query); got != want {
			t.Errorf("%s = %q, want %q", query, got, want)
		}
	}
	// The identity goes on from where it was.
	if id := queryString(t, connection, "INSERT INTO notes (company_id, body) SELECT id, 'after' FROM companies LIMIT 1 RETURNING id::text"); id != "151" {
		t.Errorf("the next note's id is %s, want 151", id)
	}

	if _, err := os.Stat(filepath.Join(dir, oldMajor, "PG_VERSION")); err != nil {
		t.Fatalf("the old cluster isn't kept: %v", err)
	}
	assertMissing(t, filepath.Join(dir, newMajor+".partial"))
	dumps := listNames(t, backups)
	if len(dumps) != 1 || !strings.HasPrefix(dumps[0], "hub-pre-upgrade-17-to-18-") || !strings.HasSuffix(dumps[0], ".dump") {
		t.Fatalf("backups = %v, want one hub-pre-upgrade-17-to-18-<date>.dump", dumps)
	}

	// The next start runs the new cluster, with nothing to move.
	connection.Close(context.Background())
	cluster.Stop()
	again, err := startUpgrading(t, dir, bothEngines(t), backups)
	if err != nil {
		t.Fatal(err)
	}
	if again.Major() != 18 || again.UpgradeError() != nil {
		t.Fatalf("the next start runs Postgres %d: %v", again.Major(), again.UpgradeError())
	}
	if got := queryString(t, connect(t, again), "SELECT count(*)::text FROM notes"); got != "151" {
		t.Fatalf("after a restart, notes has %s rows", got)
	}
	if dumps := listNames(t, backups); len(dumps) != 1 {
		t.Fatalf("a start with nothing to move dumped: %v", dumps)
	}
}

// failingRestore is a database whose dump restores only partway: the check
// on guarded calls a function that names allowed without its schema, and
// pg_restore runs with an empty search_path, so loading guarded's rows fails.
const failingRestore = `
CREATE TABLE allowed (note text);
INSERT INTO allowed VALUES ('kept');
CREATE FUNCTION is_allowed(candidate text) RETURNS boolean LANGUAGE plpgsql
	AS $$ BEGIN RETURN EXISTS (SELECT 1 FROM allowed WHERE note = candidate); END $$;
CREATE TABLE guarded (note text CHECK (is_allowed(note)));
INSERT INTO guarded VALUES ('kept');
`

// fixedRestore names allowed's schema, so the dump restores.
const fixedRestore = `
CREATE OR REPLACE FUNCTION is_allowed(candidate text) RETURNS boolean LANGUAGE plpgsql
	AS $$ BEGIN RETURN EXISTS (SELECT 1 FROM public.allowed WHERE note = candidate); END $$;
`

func TestAnUpgradeWhoseRestoreFailsRunsTheOldClusterAndTheNextStartTriesAgain(t *testing.T) {
	dir := oldCluster(t, failingRestore)
	backups := t.TempDir()

	cluster, err := startUpgrading(t, dir, bothEngines(t), backups)
	if err != nil {
		t.Fatalf("the server didn't fall back to the old cluster: %v", err)
	}
	failure := cluster.UpgradeError()
	if failure == nil || failure.From != 17 || failure.To != 18 || !strings.Contains(failure.Error(), "pg_restore") {
		t.Fatalf("UpgradeError = %v, want a failed pg_restore from 17 to 18", failure)
	}
	if cluster.Major() != 17 || cluster.DataDir() != filepath.Join(dir, oldMajor) {
		t.Fatalf("running Postgres %d in %s, want the old cluster", cluster.Major(), cluster.DataDir())
	}
	connection := connect(t, cluster)
	if version := queryString(t, connection, "SELECT (current_setting('server_version_num')::int / 10000)::text"); version != oldMajor {
		t.Fatalf("the server is Postgres %s", version)
	}
	if note := queryString(t, connection, "SELECT note FROM guarded"); note != "kept" {
		t.Fatalf("the old cluster serves %q", note)
	}
	assertMissing(t, filepath.Join(dir, newMajor+".partial"))
	assertMissing(t, filepath.Join(dir, newMajor))

	// Once the database restores, the next start moves it.
	execute(t, connection, fixedRestore)
	connection.Close(context.Background())
	cluster.Stop()
	again, err := startUpgrading(t, dir, bothEngines(t), backups)
	if err != nil {
		t.Fatal(err)
	}
	if again.Major() != 18 || again.UpgradeError() != nil {
		t.Fatalf("the next start runs Postgres %d: %v", again.Major(), again.UpgradeError())
	}
	if note := queryString(t, connect(t, again), "SELECT note FROM guarded"); note != "kept" {
		t.Fatalf("after the upgrade: %q", note)
	}
}

func TestAPartialClusterAKilledUpgradeLeftIsRemovedAndTheUpgradeCompletes(t *testing.T) {
	dir := oldCluster(t, "CREATE TABLE kept (note text); INSERT INTO kept VALUES ('moved')")
	// What an upgrade killed mid-way leaves: a cluster half made by initdb,
	// or half restored.
	partial := filepath.Join(dir, newMajor+".partial")
	if err := os.MkdirAll(filepath.Join(partial, "base", "1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partial, "PG_VERSION"), []byte(newMajor+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cluster, err := startUpgrading(t, dir, bothEngines(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cluster.Major() != 18 || cluster.UpgradeError() != nil {
		t.Fatalf("running Postgres %d: %v", cluster.Major(), cluster.UpgradeError())
	}
	if note := queryString(t, connect(t, cluster), "SELECT note FROM kept"); note != "moved" {
		t.Fatalf("after the upgrade: %q", note)
	}
	assertMissing(t, partial)
}

func TestAnUpgradeWithoutTheOldEngineIsRefusedNamingTheNewestDump(t *testing.T) {
	dir := oldCluster(t, "CREATE TABLE kept (note text)")
	backups := t.TempDir()
	written := time.Now().Add(-48 * time.Hour)
	for order, name := range []string{"hub-2026-10-01.dump", "hub-pre-migration-80.dump"} {
		path := filepath.Join(backups, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := written.Add(time.Duration(order) * time.Hour)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	onlyNew := newEngines(t, map[string]string{newMajor: getEngine(t)})

	_, err := startUpgrading(t, dir, onlyNew, backups)
	newest := filepath.Join(backups, "hub-pre-migration-80.dump")
	if err == nil || !strings.Contains(err.Error(), "no Postgres 17 engine") || !strings.Contains(err.Error(), "hub-server database restore '"+newest+"'") {
		t.Fatalf("err = %v, want a refusal naming %s", err, newest)
	}
	assertMissing(t, filepath.Join(dir, newMajor))
	assertMissing(t, filepath.Join(dir, newMajor+".partial"))
	if _, err := os.Stat(filepath.Join(dir, oldMajor, "PG_VERSION")); err != nil {
		t.Fatalf("the old cluster changed: %v", err)
	}
	if dumps := listNames(t, backups); len(dumps) != 2 {
		t.Fatalf("the refusal dumped: %v", dumps)
	}

	// With no dump at all, it says to install a release with the old engine.
	_, err = startUpgrading(t, dir, onlyNew, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "install a hub release that ships Postgres 17") {
		t.Fatalf("with no dump: %v", err)
	}
}
