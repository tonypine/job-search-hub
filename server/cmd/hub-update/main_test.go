package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
)

func TestVersionPrintsTheHubsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if want := "hub-update " + buildinfo.Version() + "\n"; stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestAnythingElsePrintsTheUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "hub-update install <state.json>") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestAnInstallWithoutItsStateStopsForLaunchdToRunAgain(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := run([]string{"install", filepath.Join(t.TempDir(), "state.json")}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "state.json") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}

func TestTheHubIsReachedOnHubAddrsPort(t *testing.T) {
	for address, want := range map[string]string{"127.0.0.1:8091": "http://127.0.0.1:8091", ":8092": "http://127.0.0.1:8092", "": "http://127.0.0.1:8090"} {
		if got := makeHubURL(address); got != want {
			t.Errorf("makeHubURL(%q) = %q, want %q", address, got, want)
		}
	}
}

func TestTheServerLogIsReadInWholeLines(t *testing.T) {
	folder := t.TempDir()
	system := &launchdSystem{serverLog: filepath.Join(folder, "server.log")}
	if lines, next := system.ReadServerLog(0); lines != nil || next != 0 {
		t.Fatalf("no log: %q, %d", lines, next)
	}
	if err := os.WriteFile(system.serverLog, []byte("one\ntwo\nthr"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, next := system.ReadServerLog(0)
	if strings.Join(lines, "|") != "one|two" || next != 8 {
		t.Fatalf("lines %q, next %d", lines, next)
	}
	if err := os.WriteFile(system.serverLog, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if lines, next = system.ReadServerLog(next); strings.Join(lines, "|") != "three" || next != 14 {
		t.Fatalf("lines %q, next %d", lines, next)
	}
	if system.ServerLogEnd() != 14 {
		t.Fatalf("end = %d", system.ServerLogEnd())
	}
}
