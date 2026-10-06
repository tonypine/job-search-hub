package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheServerReadsItsSettingsFileWithoutOverridingTheEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.env")
	content := `# hub-server's settings
HUB_ADDR=127.0.0.1:8090

export HUB_PUBLIC_URL="http://localhost:8090"
HUB_JOB_FACTS_MODEL='qwen/qwen3.5-9b'
HUB_CV_FOLDER=/Users/ada/CVs # not a comment
HUB_EMPTY=
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{"HUB_ADDR": ":9000"}
	lookup := func(name string) (string, bool) { value, ok := environment[name]; return value, ok }
	set := func(name, value string) error { environment[name] = value; return nil }
	if err := loadEnvFile(path, lookup, set); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"HUB_ADDR":            ":9000",
		"HUB_PUBLIC_URL":      "http://localhost:8090",
		"HUB_JOB_FACTS_MODEL": "qwen/qwen3.5-9b",
		"HUB_CV_FOLDER":       "/Users/ada/CVs # not a comment",
		"HUB_EMPTY":           "",
	}
	for name, value := range want {
		if environment[name] != value {
			t.Errorf("%s = %q, want %q", name, environment[name], value)
		}
	}
	if len(environment) != len(want) {
		t.Errorf("environment = %v", environment)
	}
}

func TestAMissingSettingsFileSetsNothingAndABrokenOneFails(t *testing.T) {
	set := func(name, value string) error { t.Fatalf("set %s", name); return nil }
	if err := loadEnvFile(filepath.Join(t.TempDir(), "server.env"), os.LookupEnv, set); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	path := filepath.Join(t.TempDir(), "server.env")
	if err := os.WriteFile(path, []byte("HUB_ADDR=:8090\nnot a setting\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadEnvFile(path, func(string) (string, bool) { return "", false }, func(string, string) error { return nil }); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("broken file: %v", err)
	}
}

func TestTheServersPathPutsHomebrewAndTheUsersToolsFirst(t *testing.T) {
	got := makeSearchPath("/usr/bin:/bin:/usr/sbin:/sbin:/Users/ada/go/bin", "/Users/ada")
	want := "/opt/homebrew/bin:/usr/local/bin:/Users/ada/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/Users/ada/go/bin"
	if got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
}
