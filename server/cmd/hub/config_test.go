package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lookupFrom(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func writeConfigFile(t *testing.T, home, contents string, mode os.FileMode) {
	t.Helper()
	directory := filepath.Join(home, ".config", "job-search-hub")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func TestReadConfigPrefersTheEnvironment(t *testing.T) {
	home := t.TempDir()
	writeConfigFile(t, home, `{"url":"http://file:1","owner_token":"from-file"}`, 0o600)

	config, err := readConfig(lookupFrom(map[string]string{"HUB_OWNER_TOKEN": "from-env"}), home)
	if err != nil || config.OwnerToken != "from-env" || config.HubURL != "http://file:1" {
		t.Fatalf("config = %+v, err = %v", config, err)
	}
}

func TestReadConfigDefaultsTheURLAndNeedsAToken(t *testing.T) {
	home := t.TempDir()

	config, err := readConfig(lookupFrom(map[string]string{"HUB_OWNER_TOKEN": "token"}), home)
	if err != nil || config.HubURL != defaultHubURL {
		t.Fatalf("config = %+v, err = %v", config, err)
	}
	if _, err := readConfig(lookupFrom(nil), home); err == nil || !strings.Contains(err.Error(), "HUB_OWNER_TOKEN") {
		t.Fatalf("err = %v, want a missing token error", err)
	}
}

func TestReadConfigRefusesAFileOthersCanRead(t *testing.T) {
	home := t.TempDir()
	writeConfigFile(t, home, `{"owner_token":"from-file"}`, 0o644)

	if _, err := readConfig(lookupFrom(nil), home); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("err = %v, want a permissions error", err)
	}
}
