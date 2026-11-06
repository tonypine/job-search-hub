package hubupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const wentBack = "You went back from 0.1.252 to 0.1.247."

// prepareGoingBack lays out the folders as the app leaves them when the
// owner goes back from the version running to the one at oldApp: the newer
// version installed and answering, and, after a migration, the dump the
// newer server took the day before.
func (hub *fakeHub) prepareGoingBack(oldApp string, migrated bool) State {
	hub.t.Helper()
	writeBundle(hub.t, hub.installed(), newVersion)
	writeBundle(hub.t, oldApp, oldVersion)
	if err := os.WriteFile(hub.jobPlist(), []byte("plist"), 0o600); err != nil {
		hub.t.Fatal(err)
	}
	hub.answering = newVersion
	hub.appRunning = true
	state := State{
		From: oldVersion, To: newVersion, Installed: hub.installed(), NewApp: oldApp, ReopenApp: true, GoingBack: true,
		FromMigration: 88, ToMigration: 88, StartedAt: time.Now(), JobPlist: hub.jobPlist(), Step: StepWaitingForApp, Failure: wentBack,
	}
	if migrated {
		state.ToMigration = 91
		dump := hub.dump()
		if err := os.WriteFile(dump, []byte("dump"), 0o600); err != nil {
			hub.t.Fatal(err)
		}
		yesterday := time.Now().Add(-24 * time.Hour)
		if err := os.Chtimes(dump, yesterday, yesterday); err != nil {
			hub.t.Fatal(err)
		}
		hub.marked = true
		hub.lost = "Lost: 3 jobs found and 1 card moved."
	}
	hub.save(state)
	return state
}

func (hub *fakeHub) dump() string {
	return filepath.Join(hub.backups, "hub-pre-migration-88.dump")
}

func (hub *fakeHub) readInstalls() []Install {
	hub.t.Helper()
	var installs []Install
	data, _ := os.ReadFile(filepath.Join(hub.updates, InstallsFile))
	_ = json.Unmarshal(data, &installs)
	return installs
}

// requireWentBack checks a finished going back: the earlier version in place
// and answering, the one left marked bad and gone, the app reopened on the
// earlier one, the feed not told, and the going back recorded.
func (hub *fakeHub) requireWentBack(state State, lost, dump string) {
	hub.t.Helper()
	if state.Step != StepRolledBack {
		hub.t.Fatalf("ended at %s (%s %s), want rolled back; calls %v", state.Step, state.Failure, state.Error, hub.calls)
	}
	if state.Failure != wentBack || state.Lost != lost || state.Dump != dump {
		hub.t.Fatalf("failure %q, lost %q, dump %q; want %q, %q, %q", state.Failure, state.Lost, state.Dump, wentBack, lost, dump)
	}
	if !isVersion(hub.installed(), oldVersion) || hub.answering != oldVersion {
		hub.t.Fatalf("installed %s answering %q, want the earlier version", hub.installed(), hub.answering)
	}
	if isVersion(state.NewApp, newVersion) || isVersion(hub.previous(), newVersion) {
		hub.t.Fatal("the version left is still on this Mac")
	}
	data, _ := os.ReadFile(filepath.Join(hub.updates, BadVersionsFile))
	if !strings.Contains(string(data), newVersion) {
		hub.t.Fatalf("%s isn't marked bad: %s", newVersion, data)
	}
	if hub.called("post update") != 0 || hub.called("open app "+oldVersion) != 1 || hub.called("stop server") != 1 {
		hub.t.Fatalf("calls = %v", hub.calls)
	}
	installs := hub.readInstalls()
	if len(installs) != 1 || installs[0].From != newVersion || installs[0].To != oldVersion || !installs[0].WentBack || installs[0].Dump != dump {
		hub.t.Fatalf("installs = %+v", installs)
	}
	if _, err := os.Stat(hub.jobPlist()); err == nil {
		hub.t.Fatal("the launchd job's plist is still there")
	}
}

func TestGoingBackWithoutAMigrationSinceSwapsTheBundleBackAndLosesNothing(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepareGoingBack(hub.previous(), false)
	hub.appQuitsAfter = 1

	hub.requireWentBack(hub.run(), "Nothing was lost.", "")
	if hub.called("count writes") != 0 || hub.called("restore") != 0 {
		t.Fatalf("going back without a migration counted or restored: %v", hub.calls)
	}
}

func TestGoingBackOverAMigrationRestoresItsDumpAndSaysWhatItLost(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepareGoingBack(hub.previous(), true)

	hub.requireWentBack(hub.run(), "Lost: 3 jobs found and 1 card moved.", hub.dump())
	// Counted by the version left, which wrote the mark; restored by the
	// earlier one, before it's put back.
	if hub.called("count writes "+CommandPath(hub.installed(), "hub-server")+" 88") != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
	if hub.called("restore "+CommandPath(hub.previous(), "hub-server")+" "+hub.dump()) != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
	order := slices.IndexFunc(hub.calls, func(call string) bool { return strings.HasPrefix(call, "restore") })
	if order < 0 || order > slices.Index(hub.calls, "start server "+oldVersion) {
		t.Fatalf("the database was restored after the earlier server started: %v", hub.calls)
	}
}

func TestGoingBackOverAMigrationWithoutAMarkSaysItCouldntCount(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepareGoingBack(hub.previous(), true)
	hub.marked = false
	hub.lost = "Nothing was lost."

	hub.requireWentBack(hub.run(), "What was written since 0.1.252 changed the database couldn't be counted, and was lost.", hub.dump())
}

func TestGoingBackWithoutItsDumpChangesNothing(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepareGoingBack(hub.previous(), true)
	if err := os.Remove(hub.dump()); err != nil {
		t.Fatal(err)
	}

	state := hub.run()
	if state.Step != StepAbandoned || state.Failure != "The copy of the database from before 0.1.252 changed it is gone, so the hub stayed on 0.1.252." {
		t.Fatalf("state = %+v", state)
	}
	if !isVersion(hub.installed(), newVersion) || !isVersion(hub.previous(), oldVersion) || hub.answering != newVersion {
		t.Fatal("something changed")
	}
	if hub.called("restore") != 0 || hub.called("open app "+newVersion) != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
	if data, _ := os.ReadFile(filepath.Join(hub.updates, BadVersionsFile)); len(data) != 0 {
		t.Fatalf("marked bad: %s", data)
	}
}

func TestGoingBackWhenTheServerDoesntStopChangesNothing(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepareGoingBack(hub.previous(), true)
	hub.stopFails = true

	state := hub.run()
	if state.Step != StepAbandoned || state.Failure != "The server didn't stop, so the hub stayed on 0.1.252." {
		t.Fatalf("state = %+v", state)
	}
	if !isVersion(hub.installed(), newVersion) || hub.called("count writes") != 0 {
		t.Fatalf("something changed: %v", hub.calls)
	}
}

func TestGoingBackThatKeepsStoppingBeforeTheRestoreChangesNothing(t *testing.T) {
	hub := newFakeHub(t)
	state := hub.prepareGoingBack(hub.previous(), true)
	hub.answering = ""
	state.Step = StepCountingWrites
	state.Runs = DefaultMaxRuns
	hub.save(state)

	state = hub.run()
	if state.Step != StepAbandoned || state.Failure != "hub-update kept stopping before it went back to 0.1.247, so the hub stayed on 0.1.252." {
		t.Fatalf("state = %+v", state)
	}
	if !isVersion(hub.installed(), newVersion) || hub.answering != newVersion || hub.called("restore") != 0 {
		t.Fatalf("calls = %v", hub.calls)
	}
}

func TestGoingBackWhoseEarlierServerDoesntComeBackSaysHowToFinish(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepareGoingBack(hub.previous(), true)
	hub.failing[oldVersion] = true

	state := hub.run()
	if state.Step != StepRollbackFailed || state.Failure != "The hub couldn't go back to 0.1.247 by itself." {
		t.Fatalf("state = %+v", state)
	}
	if !strings.Contains(state.Error, "checking_old_server") || len(state.Commands) == 0 {
		t.Fatalf("error %q, commands %v", state.Error, state.Commands)
	}
}

func TestGoingBackToAnOlderDownloadWaitsForItToMigrateUpFromTheDump(t *testing.T) {
	hub := newFakeHub(t)
	download := filepath.Join(hub.updates, oldVersion, "JobSearchHub.app")
	hub.prepareGoingBack(download, true)
	// The earlier version knows a migration past the dump's: it migrates for
	// longer than the server's wait, and its progress moves the wait on.
	hub.slow[oldVersion] = true

	hub.requireWentBack(hub.run(), "Lost: 3 jobs found and 1 card moved.", hub.dump())
	if _, err := os.Stat(filepath.Dir(download)); !os.IsNotExist(err) {
		t.Fatalf("the download's folder is still there: %v", err)
	}
	if hub.called("restore "+CommandPath(download, "hub-server")+" "+hub.dump()) != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
}
