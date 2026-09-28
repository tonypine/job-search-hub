package main

import (
	"strings"
	"testing"
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
