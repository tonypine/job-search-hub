package main

import (
	"strings"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

var validEnvironment = map[string]string{
	"HUB_ADDR":         ":8090",
	"HUB_DATABASE_URL": "postgres://hub:secret@localhost:5434/hub",
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
	for _, name := range []string{"HUB_DATABASE_URL", "HUB_OWNER_TOKEN"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
	if strings.Contains(err.Error(), "HUB_ADDR") {
		t.Errorf("error %q names HUB_ADDR, which is set", err)
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
	for raw, want := range map[string]time.Duration{"": time.Hour, "30m": 30 * time.Minute, "0": 0} {
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
	if err != nil || parsed.jobFactsModelURL != "" || parsed.jobFactsModel != defaultJobFactsModel || parsed.jobFactsInterval != defaultJobFactsInterval {
		t.Fatalf("defaults = %+v, %v", parsed, err)
	}

	environment := map[string]string{"HUB_JOB_FACTS_MODEL_URL": "http://host.docker.internal:1234/v1", "HUB_JOB_FACTS_MODEL": "other", "HUB_JOB_FACTS_INTERVAL": "2m"}
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
