package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestOnlyComposeYamlsAddressIsTheDatabaseToMove(t *testing.T) {
	for databaseURL, want := range map[string]bool{
		"postgres://hub:0123abcd@localhost:5434/hub":                                     true,
		"postgresql://hub:0123abcd@127.0.0.1:5434/hub":                                   true,
		"postgres://hub:0123abcd@[::1]:5434/hub":                                         true,
		"postgres://hub@localhost:5434/hub":                                              true,
		"postgres://hub:0123abcd@localhost:5432/hub":                                     false,
		"postgres://hub:0123abcd@db.example:5434/hub":                                    false,
		"postgres://other:0123abcd@localhost:5434/hub":                                   false,
		"postgres://hub:0123abcd@localhost:5434/other":                                   false,
		"postgres://localhost:5434/hub":                                                  false,
		"postgres://hub:pw@hub.abc.us-east-1.rds.amazonaws.com:5432/hub?sslmode=require": false,
		"host=localhost port=5434 user=hub dbname=hub":                                   false,
		"": false,
	} {
		if got := isComposeDatabase(databaseURL); got != want {
			t.Errorf("isComposeDatabase(%q) = %v, want %v", databaseURL, got, want)
		}
	}
}

func TestServerEnvIsWrittenWithoutTheDatabaseVariablesAndEveryOtherByteKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.env")
	settings := "# hub-server's settings\n" +
		"HUB_ADDR=127.0.0.1:8090\n" +
		"HUB_DATABASE_URL=postgres://hub:secret@localhost:5434/hub\n" +
		"# HUB_DATABASE_URL=kept, a comment\n" +
		"\n" +
		"  export HUB_DATABASE_PASSWORD='secret'\n" +
		"HUB_OWNER_TOKEN=\"token=with=equals\"\n" +
		"HUB_DATABASE_URL_NOTE=kept\n" +
		"HUB_DATABASE_URL=postgres://hub:other@localhost:5434/hub\n" +
		"HUB_JOB_FACTS_MODEL=qwen/qwen3.5-9b"
	if err := os.WriteFile(path, []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeEnvFileWithout(path, []byte(settings), composeVariables); err != nil {
		t.Fatal(err)
	}
	want := "# hub-server's settings\n" +
		"HUB_ADDR=127.0.0.1:8090\n" +
		"# HUB_DATABASE_URL=kept, a comment\n" +
		"\n" +
		"HUB_OWNER_TOKEN=\"token=with=equals\"\n" +
		"HUB_DATABASE_URL_NOTE=kept\n" +
		"HUB_JOB_FACTS_MODEL=qwen/qwen3.5-9b"
	if got := readFile(t, path); got != want {
		t.Errorf("server.env:\n%s\nwant:\n%s", got, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("server.env's mode: %v, %v", info, err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("the folder holds %d files, want server.env alone", len(entries))
	}
}

func TestTheMoveCommandMovesNothingFromAServerEnvWithoutADatabaseURL(t *testing.T) {
	folder := t.TempDir()
	envFile := filepath.Join(folder, "server.env")
	settings := "HUB_ADDR=127.0.0.1:8090\nHUB_OWNER_TOKEN=token\n"
	if err := os.WriteFile(envFile, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	lookup := lookupFrom(map[string]string{
		"HUB_POSTGRES_ENGINES": filepath.Join(folder, "engines"),
		"HUB_POSTGRES_DIR":     filepath.Join(folder, "postgres"),
		"HUB_BACKUPS_DIR":      filepath.Join(folder, "backups"),
	})
	var out bytes.Buffer
	if err := runDatabaseCommand([]string{"move-from-compose", envFile}, lookup, &out); err != nil {
		t.Fatalf("move-from-compose: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "No database to move") {
		t.Errorf("the output doesn't say nothing moved:\n%s", out.String())
	}
	if got := readFile(t, envFile); got != settings {
		t.Errorf("server.env changed:\n%s", got)
	}
	if entries, _ := os.ReadDir(folder); len(entries) != 1 {
		t.Errorf("the move created %d files beside server.env", len(entries)-1)
	}
}

// composeServerEnv writes a server.env, as one from before the server owned
// its database, whose HUB_DATABASE_URL is source, and returns its path.
func composeServerEnv(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.env")
	settings := "# hub-server's settings\nHUB_ADDR=127.0.0.1:8090\nHUB_DATABASE_URL=" + source + "\nHUB_DATABASE_PASSWORD=secret\nHUB_OWNER_TOKEN=0123456789abcdef0123456789abcdef\n"
	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// startFromServerEnv opens the database a server started with the
// server.env at path would, with the owned database in settings' folders.
func startFromServerEnv(t *testing.T, path string, settings config) *hubDatabase {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	variables, err := parseEnvFile(file)
	if err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{}
	for _, variable := range variables {
		if _, set := environment[variable[0]]; !set {
			environment[variable[0]] = variable[1]
		}
	}
	settings.databaseURL = environment["HUB_DATABASE_URL"]
	return openOwnedDatabase(t, settings)
}

// testMove is a move whose old database is source, in settings' folders.
func testMove(t *testing.T, settings config, source string) (databaseMove, string) {
	backups := t.TempDir()
	return databaseMove{
		engines: settings.postgresEngines, dir: settings.postgresDir, backups: backups,
		isOld: func(databaseURL string) bool { return databaseURL == source },
		check: checkImported,
	}, backups
}

func TestTheMoveFromComposeCopiesEveryRowAndDropsTheDatabaseURLFromServerEnv(t *testing.T) {
	// CI's Postgres stands in for compose.yaml's.
	source := externalDatabaseURL(t)
	execute(t, source, "INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example'), ('beta', 'beta.example'), ('Gamma', 'gamma.example')")
	before := mustCountRows(t, source)
	envFile := composeServerEnv(t, source)
	settings, _ := ownedDatabaseSettings(t)
	move, backups := testMove(t, settings, source)

	var out bytes.Buffer
	moved, err := move.run(context.Background(), envFile, &out)
	if err != nil || !moved {
		t.Fatalf("move: %v, %v\n%s", moved, err, out.String())
	}
	if want := "# hub-server's settings\nHUB_ADDR=127.0.0.1:8090\nHUB_OWNER_TOKEN=0123456789abcdef0123456789abcdef\n"; readFile(t, envFile) != want {
		t.Errorf("server.env:\n%s\nwant:\n%s", readFile(t, envFile), want)
	}
	if entries, _ := os.ReadDir(backups); len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "hub-import-") {
		t.Errorf("backups holds %v, want the import's dump", entries)
	}

	// The server starts on the database it owns, which holds every row.
	database := startFromServerEnv(t, envFile, settings)
	if database.cluster == nil {
		t.Fatal("the server didn't start its own Postgres")
	}
	if owned := mustCountRows(t, database.url); !maps.Equal(owned, before) {
		t.Errorf("the owned database's rows %v, want the source's %v", owned, before)
	}
	// The source is as it was.
	if after := mustCountRows(t, source); !maps.Equal(after, before) {
		t.Errorf("the source's rows changed from %v to %v", before, after)
	}
	if names := companyNames(t, source); names != "Acme,beta,Gamma" {
		t.Errorf("the source's companies: %s", names)
	}
}

func TestAMoveWhoseRowsDifferLeavesServerEnvAsItWasAndTheServerOnTheSource(t *testing.T) {
	source := externalDatabaseURL(t)
	execute(t, source, "INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example')")
	envFile := composeServerEnv(t, source)
	settingsBefore := readFile(t, envFile)
	settings, _ := ownedDatabaseSettings(t)
	move, _ := testMove(t, settings, source)
	// A row the source doesn't have reaches the imported database before
	// its rows are counted.
	move.check = func(ctx context.Context, source, imported string, out io.Writer) error {
		if err := executeContext(ctx, imported, "INSERT INTO companies (name, domain) VALUES ('Injected', 'injected.example')"); err != nil {
			return err
		}
		return checkImported(ctx, source, imported, out)
	}

	var out bytes.Buffer
	moved, err := move.run(context.Background(), envFile, &out)
	if err == nil || moved || !strings.Contains(err.Error(), "row counts differ from the source's in companies") {
		t.Fatalf("a move whose rows differ: %v, %v\n%s", moved, err, out.String())
	}
	if readFile(t, envFile) != settingsBefore {
		t.Fatalf("server.env changed:\n%s", readFile(t, envFile))
	}
	database := startFromServerEnv(t, envFile, settings)
	if database.cluster != nil || database.url != source {
		t.Fatalf("the server started on %s, not the source", database.url)
	}
	if count := countCompanies(t, database); count != 1 {
		t.Fatalf("the source holds %d companies, want 1", count)
	}
	database.Close()

	// Nothing was half-moved: the owned database holds nothing, and the
	// next move imports.
	move.check = checkImported
	if moved, err := move.run(context.Background(), envFile, &out); err != nil || !moved {
		t.Fatalf("the move after a failed one: %v, %v\n%s", moved, err, out.String())
	}
}

func TestADatabaseURLThatIsntComposeYamlsIsNeverMoved(t *testing.T) {
	// CI's Postgres, on 5432 with its own database, as a Postgres the owner
	// chose, then others no import is tried on.
	source := externalDatabaseURL(t)
	settings, _ := ownedDatabaseSettings(t)
	for _, databaseURL := range []string{
		source,
		"postgres://hub:secret@db.example:5434/hub",
		"postgres://hub:secret@hub.abc.us-east-1.rds.amazonaws.com:5432/hub?sslmode=require",
	} {
		envFile := composeServerEnv(t, databaseURL)
		settingsBefore := readFile(t, envFile)
		move, backups := testMove(t, settings, "")
		move.isOld = isComposeDatabase
		move.check = func(context.Context, string, string, io.Writer) error {
			t.Errorf("%s was imported", databaseURL)
			return errors.New("imported")
		}
		moved, err := move.run(context.Background(), envFile, io.Discard)
		if err != nil || moved {
			t.Errorf("%s: moved %v, %v", databaseURL, moved, err)
		}
		if readFile(t, envFile) != settingsBefore {
			t.Errorf("%s: server.env changed", databaseURL)
		}
		if entries, _ := os.ReadDir(backups); len(entries) != 0 {
			t.Errorf("%s: dumped %v", databaseURL, entries)
		}
	}
	if entries, _ := os.ReadDir(settings.postgresDir); len(entries) != 0 {
		t.Errorf("the owned database's folder holds %v", entries)
	}
}

func TestASecondMoveAfterASuccessfulOneDoesNothing(t *testing.T) {
	source := externalDatabaseURL(t)
	execute(t, source, "INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example')")
	envFile := composeServerEnv(t, source)
	settings, _ := ownedDatabaseSettings(t)
	move, backups := testMove(t, settings, source)
	if moved, err := move.run(context.Background(), envFile, io.Discard); err != nil || !moved {
		t.Fatalf("the first move: %v, %v", moved, err)
	}
	database := startFromServerEnv(t, envFile, settings)
	execute(t, database.url, "INSERT INTO companies (name, domain) VALUES ('Written after the move', 'after.example')")
	database.Close()
	settingsAfter := readFile(t, envFile)
	dumps, _ := os.ReadDir(backups)

	move.check = func(context.Context, string, string, io.Writer) error {
		t.Error("the second move imported again")
		return errors.New("imported")
	}
	var out bytes.Buffer
	if moved, err := move.run(context.Background(), envFile, &out); err != nil || moved {
		t.Fatalf("the second move: %v, %v\n%s", moved, err, out.String())
	}
	if readFile(t, envFile) != settingsAfter {
		t.Errorf("server.env changed:\n%s", readFile(t, envFile))
	}
	if again, _ := os.ReadDir(backups); len(again) != len(dumps) {
		t.Errorf("backups went from %v to %v", dumps, again)
	}
	clusters, _ := os.ReadDir(settings.postgresDir)
	for _, cluster := range clusters {
		if strings.Contains(cluster.Name(), ".replaced-") {
			t.Errorf("the owned database was replaced: %s", cluster.Name())
		}
	}
	if count := countCompanies(t, startFromServerEnv(t, envFile, settings)); count != 2 {
		t.Errorf("the owned database holds %d companies, want the moved one and the one written after", count)
	}
}

func TestAMoveAfterAnInstallRolledOneBackReplacesTheLeftoverCopy(t *testing.T) {
	source := externalDatabaseURL(t)
	execute(t, source, "INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example')")
	envFile := composeServerEnv(t, source)
	settingsBefore := readFile(t, envFile)
	settings, _ := ownedDatabaseSettings(t)
	move, _ := testMove(t, settings, source)
	if moved, err := move.run(context.Background(), envFile, io.Discard); err != nil || !moved {
		t.Fatalf("the first move: %v, %v", moved, err)
	}
	// The new server failed its health check: install-app.sh put server.env
	// back, and the hub went on writing to the source.
	if err := os.WriteFile(envFile, []byte(settingsBefore), 0o600); err != nil {
		t.Fatal(err)
	}
	execute(t, source, "INSERT INTO companies (name, domain) VALUES ('Written after the rollback', 'rollback.example')")
	before := mustCountRows(t, source)

	var out bytes.Buffer
	if moved, err := move.run(context.Background(), envFile, &out); err != nil || !moved {
		t.Fatalf("the move after the rollback: %v, %v\n%s", moved, err, out.String())
	}
	if want := "# hub-server's settings\nHUB_ADDR=127.0.0.1:8090\nHUB_OWNER_TOKEN=0123456789abcdef0123456789abcdef\n"; readFile(t, envFile) != want {
		t.Errorf("server.env:\n%s\nwant:\n%s", readFile(t, envFile), want)
	}
	if !strings.Contains(out.String(), "The database it replaced is kept in") {
		t.Errorf("the output doesn't say where the leftover copy is kept:\n%s", out.String())
	}
	replaced := 0
	clusters, _ := os.ReadDir(settings.postgresDir)
	for _, cluster := range clusters {
		if strings.Contains(cluster.Name(), ".replaced-") {
			replaced++
		}
	}
	if replaced != 1 {
		t.Errorf("the owned database's folder holds %v, want the leftover copy kept once", clusters)
	}
	database := startFromServerEnv(t, envFile, settings)
	if database.cluster == nil {
		t.Fatal("the server didn't start its own Postgres")
	}
	if owned := mustCountRows(t, database.url); !maps.Equal(owned, before) {
		t.Errorf("the owned database's rows %v, want the source's %v", owned, before)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func mustCountRows(t *testing.T, databaseURL string) map[string]int64 {
	t.Helper()
	counts, err := countRows(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func companyNames(t *testing.T, databaseURL string) string {
	t.Helper()
	connection, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	var names string
	if err := connection.QueryRow(context.Background(), "SELECT string_agg(name, ',' ORDER BY id) FROM companies").Scan(&names); err != nil {
		t.Fatal(err)
	}
	return names
}

func executeContext(ctx context.Context, databaseURL, statement string) error {
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer connection.Close(context.Background())
	_, err = connection.Exec(ctx, statement)
	return err
}
