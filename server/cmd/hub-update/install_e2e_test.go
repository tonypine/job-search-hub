package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/hubupdate"
	"github.com/tonypine/job-search-hub/server/internal/serverenv"
)

// The end-to-end test runs hub-update install against a home folder that
// holds two built bundles, each with a test hub-server (testdata/testhub),
// with stand-ins for the Mac's launchctl, pgrep, pkill and open
// (testdata/bin) that run the installed bundle's server as launchd would.

const (
	e2eOld     = "0.1.247"
	e2eNew     = "0.1.252"
	e2eToken   = "e2e-owner-token"
	e2eRecords = "rows written on " + e2eOld + "\n"
)

var (
	testHubs      = map[string]string{}
	testHubsMutex sync.Mutex
)

// buildTestHub builds testdata/testhub as version's hub-server in mode,
// once per test binary.
func buildTestHub(t *testing.T, version, mode string) string {
	t.Helper()
	testHubsMutex.Lock()
	defer testHubsMutex.Unlock()
	key := version + "-" + mode
	if built, ok := testHubs[key]; ok {
		return built
	}
	goCommand, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the end-to-end test builds its servers with go: %v", err)
	}
	out := filepath.Join(os.TempDir(), fmt.Sprintf("hub-update-e2e-%d", os.Getpid()), "hub-server-"+key)
	command := exec.Command(goCommand, "build", "-o", out, "-ldflags", fmt.Sprintf("-X main.version=%s -X main.mode=%s", version, mode), "./testdata/testhub")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build the test hub-server: %v\n%s", err, output)
	}
	testHubs[key] = out
	return out
}

func TestMain(m *testing.M) {
	code := m.Run()
	_ = os.RemoveAll(filepath.Join(os.TempDir(), fmt.Sprintf("hub-update-e2e-%d", os.Getpid())))
	os.Exit(code)
}

// e2eHome is a home folder as the app leaves it when it starts an install:
// the old version installed and its server running, the new one
// downloaded, and the install's state at its first step.
type e2eHome struct {
	t       *testing.T
	home    string
	support string
	updates string
	hubURL  string
	state   hubupdate.State
}

func newE2EHome(t *testing.T, newMode string) *e2eHome {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	support := filepath.Join(home, "Library", "Application Support", "JobSearchHub")
	updates := filepath.Join(support, "Updates")
	port := findFreePort(t)
	e2e := &e2eHome{t: t, home: home, support: support, updates: updates, hubURL: fmt.Sprintf("http://127.0.0.1:%d", port)}

	standIns, err := filepath.Abs(filepath.Join("testdata", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	saved := macCommands
	macCommands.launchctl = filepath.Join(standIns, "launchctl")
	macCommands.pgrep = filepath.Join(standIns, "pgrep")
	macCommands.pkill = filepath.Join(standIns, "pkill")
	macCommands.open = filepath.Join(standIns, "open")
	t.Cleanup(func() { macCommands = saved })

	e2e.write(serverenv.Path(home), fmt.Sprintf("HUB_ADDR=127.0.0.1:%d\nHUB_OWNER_TOKEN=%s\n", port, e2eToken))
	e2e.write(filepath.Join(support, "data.txt"), e2eRecords)
	jobPlist := filepath.Join(home, "Library", "LaunchAgents", jobLabel+".plist")
	e2e.write(jobPlist, "plist")

	installed := filepath.Join(home, "Applications", "Job Search Hub.app")
	newApp := filepath.Join(updates, e2eNew, "Job Search Hub.app")
	e2e.writeBundle(installed, e2eOld, buildTestHub(t, e2eOld, ""))
	e2e.writeBundle(newApp, e2eNew, buildTestHub(t, e2eNew, newMode))

	// The old server runs, as launchd runs it.
	e2e.launchctl("bootstrap", "gui/501", "server.plist")
	t.Cleanup(func() { e2e.launchctl("bootout", "gui/501/"+serverLabel) })
	e2e.waitForVersion(e2eOld)

	e2e.state = hubupdate.State{
		From: e2eOld, To: e2eNew, Installed: installed, NewApp: newApp, ReopenApp: true,
		FromMigration: 88, ToMigration: 91, StartedAt: time.Now().Add(-time.Second), JobPlist: jobPlist, Step: hubupdate.StepWaitingForApp,
	}
	e2e.save()
	return e2e
}

func findFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func (e2e *e2eHome) write(path, content string) {
	e2e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		e2e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		e2e.t.Fatal(err)
	}
}

func (e2e *e2eHome) read(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

// writeBundle lays out an app bundle of version: its Info.plist, an app
// that says it opened, and hubServer in Helpers/bin.
func (e2e *e2eHome) writeBundle(app, version, hubServer string) {
	e2e.t.Helper()
	e2e.write(filepath.Join(app, "Contents", "Info.plist"), fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>com.tonypine.jobsearchhub</string>
	<key>CFBundleShortVersionString</key>
	<string>%s</string>
</dict>
</plist>
`, version))
	// Once up and connected, the app writes its launched mark.
	e2e.write(filepath.Join(app, "Contents", "MacOS", "JobSearchHub"), fmt.Sprintf(`#!/bin/bash
echo %[1]s >> "$HOME/opened.log"
echo %[1]s > %[2]q
`, version, filepath.Join(e2e.updates, hubupdate.LaunchedMark)))
	server, err := os.ReadFile(hubServer)
	if err != nil {
		e2e.t.Fatal(err)
	}
	e2e.write(hubupdate.CommandPath(app, "hub-server"), string(server))
}

func (e2e *e2eHome) statePath() string {
	return filepath.Join(e2e.updates, "state.json")
}

func (e2e *e2eHome) save() {
	e2e.t.Helper()
	if err := e2e.state.Save(e2e.statePath()); err != nil {
		e2e.t.Fatal(err)
	}
}

func (e2e *e2eHome) launchctl(args ...string) {
	e2e.t.Helper()
	if output, err := exec.Command(macCommands.launchctl, args...).CombinedOutput(); err != nil {
		e2e.t.Fatalf("launchctl %v: %v\n%s", args, err, output)
	}
}

func (e2e *e2eHome) readVersion() string {
	response, err := http.Get(e2e.hubURL + "/v1/version")
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	var build struct {
		Version string `json:"version"`
	}
	_ = json.NewDecoder(response.Body).Decode(&build)
	return build.Version
}

func (e2e *e2eHome) waitForVersion(version string) {
	e2e.t.Helper()
	for range 100 {
		if e2e.readVersion() == version {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	e2e.t.Fatalf("the server doesn't answer %s; its log:\n%s", version, e2e.read(filepath.Join(e2e.home, "Library", "Logs", "JobSearchHub", "server.log")))
}

// install runs hub-update install, which must end the install.
func (e2e *e2eHome) install() hubupdate.State {
	e2e.t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"install", e2e.statePath()}, &stdout, &stderr); code != 0 {
		e2e.t.Fatalf("exit %d\nstdout %s\nstderr %s\ninstall.log:\n%s", code, stdout.String(), stderr.String(), e2e.read(filepath.Join(e2e.updates, "install.log")))
	}
	state, err := hubupdate.LoadState(e2e.statePath())
	if err != nil {
		e2e.t.Fatal(err)
	}
	if _, err := os.Stat(state.JobPlist); !os.IsNotExist(err) {
		e2e.t.Errorf("the job's plist is still there: %v", err)
	}
	return state
}

func (e2e *e2eHome) bundleVersion(app string) string {
	version, _ := hubupdate.BundleVersion(app)
	return version
}

// requireInstalled checks the new version is in place and running, its
// migration kept, the old one in previous/, the app reopened and the
// install recorded.
func (e2e *e2eHome) requireInstalled(state hubupdate.State) {
	e2e.t.Helper()
	if state.Step != hubupdate.StepInstalled {
		e2e.t.Fatalf("ended at %s: %s %s\ninstall.log:\n%s", state.Step, state.Failure, state.Error, e2e.read(filepath.Join(e2e.updates, "install.log")))
	}
	if got := e2e.bundleVersion(state.Installed); got != e2eNew {
		e2e.t.Errorf("installed bundle = %q", got)
	}
	if got := e2e.bundleVersion(filepath.Join(e2e.updates, "previous", "Job Search Hub.app")); got != e2eOld {
		e2e.t.Errorf("previous bundle = %q", got)
	}
	if got := e2e.readVersion(); got != e2eNew {
		e2e.t.Errorf("the server answers %q", got)
	}
	if got := e2e.read(filepath.Join(e2e.support, "data.txt")); got != e2eRecords+"migrated by "+e2eNew+"\n" {
		e2e.t.Errorf("data = %q", got)
	}
	if got := e2e.read(filepath.Join(e2e.home, "opened.log")); got != e2eNew+"\n" {
		e2e.t.Errorf("opened = %q", got)
	}
	var installs []hubupdate.Install
	if err := json.Unmarshal([]byte(e2e.read(filepath.Join(e2e.updates, "installs.json"))), &installs); err != nil || len(installs) != 1 {
		e2e.t.Fatalf("installs = %+v, %v", installs, err)
	}
	dump := filepath.Join(e2e.support, "backups", "hub-pre-migration-88.dump")
	if install := installs[0]; install.From != e2eOld || install.To != e2eNew || install.Dump != dump {
		e2e.t.Errorf("install = %+v", install)
	}
}

func TestEndToEndAnInstallPutsTheNewVersionInPlace(t *testing.T) {
	e2e := newE2EHome(t, "migrate")
	e2e.requireInstalled(e2e.install())
}

func TestEndToEndANewServerThatFailsItsCheckRollsBack(t *testing.T) {
	e2e := newE2EHome(t, "crash")

	state := e2e.install()

	if state.Step != hubupdate.StepRolledBack {
		t.Fatalf("ended at %s: %s %s\ninstall.log:\n%s", state.Step, state.Failure, state.Error, e2e.read(filepath.Join(e2e.updates, "install.log")))
	}
	failure := e2eNew + " couldn't start, so the hub went back to " + e2eOld + "."
	if state.Failure != failure || state.Lost != "Nothing was lost." {
		t.Errorf("failure %q, lost %q", state.Failure, state.Lost)
	}
	// The old bundle and its data are back, and its server runs.
	if got := e2e.bundleVersion(state.Installed); got != e2eOld {
		t.Errorf("installed bundle = %q", got)
	}
	if got := e2e.read(filepath.Join(e2e.support, "data.txt")); got != e2eRecords {
		t.Errorf("data = %q, want the dump restored", got)
	}
	if got := e2e.readVersion(); got != e2eOld {
		t.Errorf("the server answers %q", got)
	}
	// The new version is gone, marked bad, and the feed heard why.
	if _, err := os.Stat(state.NewApp); !os.IsNotExist(err) {
		t.Errorf("the new bundle is still there: %v", err)
	}
	if bad := e2e.read(filepath.Join(e2e.updates, hubupdate.BadVersionsFile)); !strings.Contains(bad, e2eNew) {
		t.Errorf("bad versions = %q", bad)
	}
	if updates := e2e.read(filepath.Join(e2e.home, "updates.jsonl")); !strings.Contains(updates, failure) || !strings.Contains(updates, "Nothing was lost.") {
		t.Errorf("feed = %q", updates)
	}
	if got := e2e.read(filepath.Join(e2e.home, "opened.log")); got != e2eOld+"\n" {
		t.Errorf("opened = %q", got)
	}
}

func TestEndToEndAnInstallCutShortMidSwapFinishes(t *testing.T) {
	e2e := newE2EHome(t, "migrate")
	// The last run stopped the server and swapped the bundles, then the Mac
	// lost power before it recorded the swap or moved the old version to
	// previous/.
	e2e.launchctl("kill", "SIGTERM", "gui/501/"+serverLabel)
	for tries := 0; e2e.readVersion() != ""; tries++ {
		if tries == 100 {
			t.Fatal("the old server didn't stop")
		}
		time.Sleep(50 * time.Millisecond)
	}
	swapped := e2e.state.Installed + ".swapping"
	for _, move := range [][2]string{{e2e.state.Installed, swapped}, {e2e.state.NewApp, e2e.state.Installed}, {swapped, e2e.state.NewApp}} {
		if err := os.Rename(move[0], move[1]); err != nil {
			t.Fatal(err)
		}
	}
	e2e.state.Step, e2e.state.Runs = hubupdate.StepSwapping, 1
	e2e.save()

	state := e2e.install()

	e2e.requireInstalled(state)
	if state.Runs != 2 {
		t.Errorf("runs = %d", state.Runs)
	}
}
