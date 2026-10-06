package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func countCompanies(t *testing.T, database *hubDatabase) int {
	t.Helper()
	var count int
	if err := database.pool.QueryRow(context.Background(), "SELECT count(*) FROM companies").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestTheImportCommandMovesAnExternalDatabasesRowsAndRefusesASecondImportWithoutReplace(t *testing.T) {
	// The hub's database in Docker's Postgres, as the owner has it today.
	source := externalDatabaseURL(t)
	execute(t, source, "INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example'), ('beta', 'beta.example')")
	settings, _ := ownedDatabaseSettings(t)
	backups := t.TempDir()
	lookup := lookupFrom(map[string]string{"HUB_POSTGRES_ENGINES": settings.postgresEngines, "HUB_POSTGRES_DIR": settings.postgresDir, "HUB_BACKUPS_DIR": backups})

	var out bytes.Buffer
	if err := runDatabaseCommand([]string{"import", source}, lookup, &out); err != nil {
		t.Fatalf("import: %v\n%s", err, out.String())
	}
	dump := filepath.Join(backups, "hub-import-"+time.Now().Format("2006-01-02")+".dump")
	if _, err := os.Stat(dump); err != nil {
		t.Fatalf("the import's dump isn't kept: %v", err)
	}
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`(?m)^table +source +imported`),
		regexp.MustCompile(`(?m)^companies +2 +2$`),
		regexp.MustCompile(`(?m)^agent_prompts +[1-9]\d* +[1-9]\d*$`),
		regexp.MustCompile(regexp.QuoteMeta(dump)),
	} {
		if !want.MatchString(out.String()) {
			t.Errorf("the output doesn't match %s:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "differs") {
		t.Errorf("a table's rows differ:\n%s", out.String())
	}

	database := openOwnedDatabase(t, settings)
	if count := countCompanies(t, database); count != 2 {
		t.Fatalf("the owned database holds %d companies, want 2", count)
	}
	execute(t, database.url, "INSERT INTO companies (name, domain) VALUES ('Gamma', 'gamma.example')")

	// Again, while the server runs, then once it's stopped: both refused.
	out.Reset()
	if err := runDatabaseCommand([]string{"import", source}, lookup, &out); err == nil || !strings.Contains(err.Error(), "stop the hub in Settings › Server first") {
		t.Fatalf("an import while the server runs: %v", err)
	}
	database.Close()
	out.Reset()
	err := runDatabaseCommand([]string{"import", source}, lookup, &out)
	if err == nil || !strings.Contains(err.Error(), "already holds data") || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("a second import without --replace: %v\n%s", err, out.String())
	}
	if count := countCompanies(t, openOwnedDatabase(t, settings)); count != 3 {
		t.Fatalf("the refused import changed the owned database: %d companies", count)
	}
}

func TestAnImportWithReplaceReplacesTheOwnedDatabaseAndKeepsIt(t *testing.T) {
	source := externalDatabaseURL(t)
	execute(t, source, "INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example')")
	settings, _ := ownedDatabaseSettings(t)
	database := openOwnedDatabase(t, settings)
	execute(t, database.url, "INSERT INTO companies (name, domain) VALUES ('Gamma', 'gamma.example'), ('Delta', 'delta.example')")
	database.Close()
	lookup := lookupFrom(map[string]string{"HUB_POSTGRES_ENGINES": settings.postgresEngines, "HUB_POSTGRES_DIR": settings.postgresDir, "HUB_BACKUPS_DIR": t.TempDir()})

	var out bytes.Buffer
	if err := runDatabaseCommand([]string{"import", source, "--replace"}, lookup, &out); err != nil {
		t.Fatalf("import --replace: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), ".replaced-") || !strings.Contains(out.String(), "delete that folder once the hub runs well") {
		t.Errorf("the output doesn't say where the replaced database is:\n%s", out.String())
	}
	if count := countCompanies(t, openOwnedDatabase(t, settings)); count != 1 {
		t.Fatalf("after the import, %d companies, want the source's 1", count)
	}
}

func TestAnImportIsRefusedWhenATablesRowsDiffer(t *testing.T) {
	source := externalDatabaseURL(t)
	execute(t, source, "INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example'), ('beta', 'beta.example')")
	imported := externalDatabaseURL(t)
	execute(t, imported, "INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example')")

	var out bytes.Buffer
	err := checkImported(context.Background(), source, imported, &out)
	if err == nil || !strings.HasSuffix(err.Error(), "row counts differ from the source's in companies") {
		t.Fatalf("differing counts: %v", err)
	}
	if !regexp.MustCompile(`(?m)^companies +2 +1 +differs$`).MatchString(out.String()) {
		t.Errorf("the output doesn't show the difference:\n%s", out.String())
	}
}

func TestRowCountsListEveryTableOfEitherDatabaseAndMarkDifferences(t *testing.T) {
	var out bytes.Buffer
	differing := printRowCounts(&out,
		map[string]int64{"companies": 12, "people": 30, "audit.notes": 1},
		map[string]int64{"companies": 12, "people": 29, "extra": 0})
	want := "" +
		"table        source  imported\n" +
		"audit.notes  1       -         differs\n" +
		"companies    12      12\n" +
		"extra        -       0         differs\n" +
		"people       30      29        differs\n"
	if out.String() != want {
		t.Errorf("printed:\n%s\nwant:\n%s", out.String(), want)
	}
	if strings.Join(differing, ",") != "audit.notes,extra,people" {
		t.Errorf("differing = %v", differing)
	}
}

func TestImportTakesASourceAndReplaceInAnyOrder(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		source    string
		replace   bool
		ok        bool
	}{
		{[]string{"postgres://hub@localhost:5434/hub"}, "postgres://hub@localhost:5434/hub", false, true},
		{[]string{"--replace", "postgres://a"}, "postgres://a", true, true},
		{[]string{"postgres://a", "--replace"}, "postgres://a", true, true},
		{[]string{"--replace"}, "", false, false},
		{[]string{"postgres://a", "postgres://b"}, "", false, false},
		{[]string{"--force", "postgres://a"}, "", false, false},
	} {
		source, replace, ok := parseImportArguments(test.arguments)
		if source != test.source || replace != test.replace || ok != test.ok {
			t.Errorf("%v: %q, %v, %v", test.arguments, source, replace, ok)
		}
	}
}
