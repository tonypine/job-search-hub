package main

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

var validEnvironment = map[string]string{
	"HUB_ADDR":         ":8090",
	"HUB_DATABASE_URL": "postgres://hub:secret@db.example:5432/hub",
	"HUB_OWNER_TOKEN":  strings.Repeat("a", 64),
}

func TestParseEnvironmentReadsEveryVariable(t *testing.T) {
	parsed, err := parseEnvironment(lookupFrom(validEnvironment))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.address != ":8090" || parsed.databaseURL != validEnvironment["HUB_DATABASE_URL"] || parsed.ownerToken != validEnvironment["HUB_OWNER_TOKEN"] {
		t.Fatalf("parsed config does not match the environment: %+v", parsed)
	}
}

func TestParseEnvironmentNamesEveryMissingVariable(t *testing.T) {
	_, err := parseEnvironment(lookupFrom(map[string]string{"HUB_ADDR": ":8090"}))
	if err == nil {
		t.Fatal("expected an error for missing variables")
	}
	if !strings.Contains(err.Error(), "HUB_OWNER_TOKEN") {
		t.Errorf("error %q does not name HUB_OWNER_TOKEN", err)
	}
	// Without a URL, the server runs its own database.
	for _, name := range []string{"HUB_ADDR", "HUB_DATABASE_URL"} {
		if strings.Contains(err.Error(), name) {
			t.Errorf("error %q names %s, which isn't missing", err, name)
		}
	}
}

func TestWithoutADatabaseURLTheServerRunsItsOwnFromTheEnginesBesideIt(t *testing.T) {
	environment := maps.Clone(validEnvironment)
	delete(environment, "HUB_DATABASE_URL")
	parsed, err := parseEnvironment(lookupFrom(environment))
	if err != nil {
		t.Fatalf("no database URL: %v", err)
	}
	folder, err := executableFolder()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	wantEngines := findEngines(folder, home)
	if parsed.databaseURL != "" || parsed.postgresEngines != wantEngines ||
		!strings.HasSuffix(parsed.postgresDir, "/Library/Application Support/JobSearchHub/postgres") {
		t.Fatalf("database %q, engines %q (want %q), folder %q", parsed.databaseURL, parsed.postgresEngines, wantEngines, parsed.postgresDir)
	}

	environment["HUB_POSTGRES_ENGINES"], environment["HUB_POSTGRES_DIR"] = "/opt/engines", "/srv/hub-postgres"
	if parsed, err = parseEnvironment(lookupFrom(environment)); err != nil || parsed.postgresEngines != "/opt/engines" || parsed.postgresDir != "/srv/hub-postgres" {
		t.Fatalf("engines %q, folder %q, err %v", parsed.postgresEngines, parsed.postgresDir, err)
	}
}

func TestTheEnginesBesideTheServerWinOverTheInstalledOnes(t *testing.T) {
	bundle, home := t.TempDir(), "/Users/ada"
	helpers := filepath.Join(bundle, "Contents", "Helpers")
	installed := "/Users/ada/Library/Application Support/JobSearchHub/engines"
	if got := findEngines(filepath.Join(helpers, "bin"), home); got != installed {
		t.Fatalf("a bundle without engines: %q, want %q", got, installed)
	}
	if err := os.MkdirAll(filepath.Join(helpers, "engines"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := findEngines(filepath.Join(helpers, "bin"), home), filepath.Join(helpers, "engines"); got != want {
		t.Fatalf("a bundle with engines: %q, want %q", got, want)
	}
}

func TestHubCVPrintIsFoundBesideTheServerUnlessSet(t *testing.T) {
	parsed, err := parseEnvironment(lookupFrom(validEnvironment))
	if err != nil {
		t.Fatal(err)
	}
	folder, err := executableFolder()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(folder, "hub-cvprint"); parsed.cvPrintCommand != want {
		t.Fatalf("cvprint %q, want %q", parsed.cvPrintCommand, want)
	}
	environment := maps.Clone(validEnvironment)
	environment["HUB_CV_PRINT_BIN"] = "/opt/hub-cvprint"
	if parsed, err = parseEnvironment(lookupFrom(environment)); err != nil || parsed.cvPrintCommand != "/opt/hub-cvprint" {
		t.Fatalf("cvprint %q, err %v", parsed.cvPrintCommand, err)
	}
}

func TestParseEnvironmentRejectsAShortOwnerToken(t *testing.T) {
	environment := map[string]string{}
	for name, value := range validEnvironment {
		environment[name] = value
	}
	environment["HUB_OWNER_TOKEN"] = "short"

	_, err := parseEnvironment(lookupFrom(environment))
	if err == nil || !strings.Contains(err.Error(), "HUB_OWNER_TOKEN") {
		t.Fatalf("expected an owner token length error, got %v", err)
	}
}

func TestParseEnvironmentReadsTheBoardPollInterval(t *testing.T) {
	for raw, want := range map[string]time.Duration{"": 15 * time.Minute, "30m": 30 * time.Minute, "0": 0} {
		environment := map[string]string{"HUB_BOARD_POLL_INTERVAL": raw}
		for name, value := range validEnvironment {
			environment[name] = value
		}
		parsed, err := parseEnvironment(lookupFrom(environment))
		if err != nil || parsed.boardPollInterval != want {
			t.Errorf("%q: interval %v, err %v; want %v", raw, parsed.boardPollInterval, err, want)
		}
	}
	environment := map[string]string{"HUB_BOARD_POLL_INTERVAL": "hourly"}
	for name, value := range validEnvironment {
		environment[name] = value
	}
	if _, err := parseEnvironment(lookupFrom(environment)); err == nil {
		t.Error("expected an error for an invalid interval")
	}
}

func TestJobFactsReadingIsOffWithoutAModelURLAndDefaultsOtherwise(t *testing.T) {
	parsed, err := parseEnvironment(lookupFrom(validEnvironment))
	if err != nil || parsed.jobFactsModelURL != "" || parsed.jobFactsModel != defaultJobFactsModel || parsed.jobFactsInterval != defaultJobFactsInterval ||
		parsed.feedPollInterval != time.Hour || parsed.boardSearchInterval != defaultBoardSearchInterval ||
		parsed.boardDiscoveryInterval != defaultBoardDiscoveryInterval {
		t.Fatalf("defaults = %+v, %v", parsed, err)
	}

	environment := map[string]string{"HUB_JOB_FACTS_MODEL_URL": "http://192.168.1.20:1234/v1", "HUB_JOB_FACTS_MODEL": "other", "HUB_JOB_FACTS_INTERVAL": "2m"}
	for name, value := range validEnvironment {
		environment[name] = value
	}
	parsed, err = parseEnvironment(lookupFrom(environment))
	if err != nil || parsed.jobFactsModelURL != environment["HUB_JOB_FACTS_MODEL_URL"] || parsed.jobFactsModel != "other" || parsed.jobFactsInterval != 2*time.Minute {
		t.Fatalf("parsed = %+v, %v", parsed, err)
	}

	environment["HUB_JOB_FACTS_INTERVAL"] = "soon"
	if _, err := parseEnvironment(lookupFrom(environment)); err == nil || !strings.Contains(err.Error(), "HUB_JOB_FACTS_INTERVAL") {
		t.Fatalf("a bad interval: err = %v", err)
	}
}

func TestGmailChangesNeedBothPubSubNamesOrNeither(t *testing.T) {
	both := map[string]string{
		"HUB_GMAIL_PUBSUB_TOPIC": "projects/p/topics/t", "HUB_GMAIL_PUBSUB_SUBSCRIPTION": "projects/p/subscriptions/s",
	}
	for name, value := range validEnvironment {
		both[name] = value
	}
	parsed, err := parseEnvironment(lookupFrom(both))
	if err != nil || parsed.gmailTopic != "projects/p/topics/t" || parsed.gmailSubscription != "projects/p/subscriptions/s" {
		t.Fatalf("parsed = %+v, %v", parsed, err)
	}

	delete(both, "HUB_GMAIL_PUBSUB_SUBSCRIPTION")
	if _, err := parseEnvironment(lookupFrom(both)); err == nil || !strings.Contains(err.Error(), "HUB_GMAIL_PUBSUB_SUBSCRIPTION") {
		t.Fatalf("a topic without a subscription: err = %v", err)
	}
}

func TestTheModelRuntimeHasDefaultsAndChecksItsPort(t *testing.T) {
	parsed, err := parseEnvironment(lookupFrom(validEnvironment))
	if err != nil || parsed.llamaServer != "llama-server" || parsed.runtimePort != defaultRuntimePort || parsed.runtimeIdleTimeout != defaultRuntimeIdleTimeout ||
		!strings.HasSuffix(parsed.modelsDir, "Application Support/JobSearchHub/models") {
		t.Fatalf("parsed = %+v, err = %v", parsed, err)
	}

	environment := maps.Clone(validEnvironment)
	environment["HUB_LLAMA_SERVER"], environment["HUB_MODELS_DIR"], environment["HUB_RUNTIME_PORT"], environment["HUB_RUNTIME_IDLE_TIMEOUT"] = "/opt/llama-server", "/models", "9000", "1m"
	parsed, err = parseEnvironment(lookupFrom(environment))
	if err != nil || parsed.llamaServer != "/opt/llama-server" || parsed.modelsDir != "/models" || parsed.runtimePort != 9000 || parsed.runtimeIdleTimeout != time.Minute {
		t.Fatalf("parsed = %+v, err = %v", parsed, err)
	}
	environment["HUB_RUNTIME_PORT"] = "eighty"
	if _, err := parseEnvironment(lookupFrom(environment)); err == nil {
		t.Fatal("a bad port was accepted")
	}
}

func TestBackupsGoToApplicationSupportWithTheDefaultPgDump(t *testing.T) {
	parsed, err := parseEnvironment(lookupFrom(validEnvironment))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(parsed.backupsDir, "/Library/Application Support/JobSearchHub/backups") || parsed.pgDump != "" {
		t.Fatalf("backups: %q with %q", parsed.backupsDir, parsed.pgDump)
	}

	environment := maps.Clone(validEnvironment)
	environment["HUB_BACKUPS_DIR"] = "/srv/hub-backups"
	environment["HUB_PG_DUMP"] = "/usr/lib/postgresql/18/bin/pg_dump"
	if parsed, _ = parseEnvironment(lookupFrom(environment)); parsed.backupsDir != "/srv/hub-backups" || parsed.pgDump != "/usr/lib/postgresql/18/bin/pg_dump" {
		t.Fatalf("backups: %q with %q", parsed.backupsDir, parsed.pgDump)
	}
}

func TestTheGoogleForJobsSearchIsOffWithoutAKeyAndKeepsToTheFreeQuota(t *testing.T) {
	parsed, err := parseEnvironment(lookupFrom(validEnvironment))
	if err != nil || parsed.jsearchAPIKey != "" || parsed.jsearchURL != "https://jsearch.p.rapidapi.com" || parsed.jsearchMonthlyRequests != 100 {
		t.Fatalf("parsed = %+v, err = %v", parsed, err)
	}

	environment := maps.Clone(validEnvironment)
	environment["HUB_JSEARCH_API_KEY"], environment["HUB_JSEARCH_URL"], environment["HUB_JSEARCH_MONTHLY_REQUESTS"] = "test-key", "https://api.example.test/jsearch", "200"
	parsed, err = parseEnvironment(lookupFrom(environment))
	if err != nil || parsed.jsearchAPIKey != "test-key" || parsed.jsearchURL != "https://api.example.test/jsearch" || parsed.jsearchMonthlyRequests != 200 {
		t.Fatalf("parsed = %+v, err = %v", parsed, err)
	}
	environment["HUB_JSEARCH_MONTHLY_REQUESTS"] = "-1"
	if _, err := parseEnvironment(lookupFrom(environment)); err == nil {
		t.Fatal("a negative request count was accepted")
	}
}
