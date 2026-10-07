package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
	"github.com/tonypine/job-search-hub/server/internal/serverenv"
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

func TestAnInstallWithoutItsStateHasNothingToRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	plist := filepath.Join(home, "Library", "LaunchAgents", jobLabel+".plist")
	if err := os.MkdirAll(filepath.Dir(plist), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plist, []byte("plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	// Exiting 0, launchd doesn't run it again, and without its plist
	// neither does the next login.
	if code := run([]string{"install", filepath.Join(t.TempDir(), "state.json")}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "No install to run") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Fatalf("the job's plist is still there: %v", err)
	}
}

func TestAStateThatCantBeUsedEndsTheJob(t *testing.T) {
	for name, content := range map[string]string{"undecodable": "{", "incomplete": `{"from":"0.1.247","step":"swapping"}`} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			plist := filepath.Join(home, "Library", "LaunchAgents", jobLabel+".plist")
			if err := os.MkdirAll(filepath.Dir(plist), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(plist, []byte("plist"), 0o600); err != nil {
				t.Fatal(err)
			}
			state := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(state, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			// No run could take it: exiting 0, launchd doesn't run it
			// again, and without its plist neither does the next login.
			if code := run([]string{"install", state}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "can't go on") {
				t.Fatalf("exit %d, stderr %q", code, stderr.String())
			}
			if _, err := os.Stat(plist); !os.IsNotExist(err) {
				t.Fatalf("the job's plist is still there: %v", err)
			}
		})
	}
}

func TestServerSettingsThatCantBeReadEndTheJob(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	env := serverenv.Path(home)
	if err := os.MkdirAll(filepath.Dir(env), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("not a setting\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"install", filepath.Join(t.TempDir(), "state.json")}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "server's settings") {
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
