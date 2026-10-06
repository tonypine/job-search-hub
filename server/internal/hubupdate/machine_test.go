package hubupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	oldVersion = "0.1.247"
	newVersion = "0.1.252"
)

// fakeHub stands in for launchd, the app and the server: the server it
// starts answers with the version of whichever bundle is installed, as
// launchd would run it.
type fakeHub struct {
	t       *testing.T
	folder  string
	updates string
	backups string
	server  *httptest.Server

	mutex sync.Mutex
	// answering is the version the server answers with; "" while it's down.
	answering string
	logLines  []string
	calls     []string

	appRunning bool
	// appQuitsAfter is how many looks the app takes to quit.
	appQuitsAfter int
	// appOpens says a reopened app writes its launched mark.
	appOpens bool

	stopFails    bool
	restoreFails bool
	// failing versions' servers never answer; crashing ones log that they
	// stopped; migrating ones dump the database first; slow ones log
	// migrations for a while before they answer.
	failing   map[string]bool
	crashing  map[string]bool
	migrating map[string]bool
	slow      map[string]bool
	// marked says the new server got as far as its mark, and lost is what
	// count-writes says it wrote since.
	marked bool
	lost   string
}

func newFakeHub(t *testing.T) *fakeHub {
	folder := t.TempDir()
	hub := &fakeHub{
		t: t, folder: folder, updates: filepath.Join(folder, "Updates"), backups: filepath.Join(folder, "backups"),
		appOpens: true, failing: map[string]bool{}, crashing: map[string]bool{}, migrating: map[string]bool{}, slow: map[string]bool{},
		lost: "Nothing was lost.",
	}
	for _, dir := range []string{hub.updates, hub.backups} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	hub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.mutex.Lock()
		answering := hub.answering
		hub.mutex.Unlock()
		if answering == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/v1/version":
			fmt.Fprintf(w, `{"version":%q,"newest_migration":91}`, answering)
		case "/v1/health":
			fmt.Fprint(w, `{"database":"ok"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(hub.server.Close)
	return hub
}

func (hub *fakeHub) installed() string {
	return filepath.Join(hub.folder, "Applications", "Job Search Hub.app")
}
func (hub *fakeHub) download() string {
	return filepath.Join(hub.updates, newVersion, "JobSearchHub.app")
}
func (hub *fakeHub) previous() string {
	return filepath.Join(hub.updates, "previous", "Job Search Hub.app")
}
func (hub *fakeHub) statePath() string {
	return filepath.Join(hub.updates, "state.json")
}
func (hub *fakeHub) jobPlist() string {
	return filepath.Join(hub.folder, "com.tonypine.jobsearchhub.update.plist")
}

// prepare lays out the folders as the app leaves them when it starts an
// install, with the old server running, and writes the state at step.
func (hub *fakeHub) prepare(migrates bool, reopen bool) State {
	hub.t.Helper()
	writeBundle(hub.t, hub.installed(), oldVersion)
	writeBundle(hub.t, hub.download(), newVersion)
	if err := os.WriteFile(hub.jobPlist(), []byte("plist"), 0o600); err != nil {
		hub.t.Fatal(err)
	}
	hub.answering = oldVersion
	hub.appRunning = reopen
	state := State{
		From: oldVersion, To: newVersion, Installed: hub.installed(), NewApp: hub.download(), ReopenApp: reopen,
		FromMigration: 88, ToMigration: 88, StartedAt: time.Now().Add(-time.Second), JobPlist: hub.jobPlist(), Step: StepWaitingForApp,
	}
	if migrates {
		state.ToMigration = 91
		hub.migrating[newVersion] = true
	}
	hub.save(state)
	return state
}

func (hub *fakeHub) save(state State) {
	hub.t.Helper()
	if err := state.Save(hub.statePath()); err != nil {
		hub.t.Fatal(err)
	}
}

func (hub *fakeHub) machine() *Machine {
	return &Machine{
		StatePath: hub.statePath(), Updates: hub.updates, Backups: hub.backups, HubURL: hub.server.URL,
		Service: "gui/501/com.tonypine.jobsearchhub.server", System: hub, Client: hub.server.Client(), Now: time.Now,
		Poll: 5 * time.Millisecond, AppQuitTimeout: 300 * time.Millisecond, ServerTimeout: 300 * time.Millisecond,
		AppTimeout: 300 * time.Millisecond, MigrationGrace: 300 * time.Millisecond, MaxRuns: DefaultMaxRuns,
	}
}

func (hub *fakeHub) run() State {
	hub.t.Helper()
	state, err := hub.machine().Run(context.Background())
	if err != nil {
		hub.t.Fatalf("run: %v", err)
	}
	saved, err := LoadState(hub.statePath())
	if err != nil || saved.Step != state.Step {
		hub.t.Fatalf("saved state = %+v, %v; the run ended at %s", saved, err, state.Step)
	}
	return state
}

func (hub *fakeHub) record(call string) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hub.calls = append(hub.calls, call)
}

func (hub *fakeHub) called(prefix string) int {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	count := 0
	for _, call := range hub.calls {
		if strings.HasPrefix(call, prefix) {
			count++
		}
	}
	return count
}

func (hub *fakeHub) IsAppRunning(context.Context) (bool, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if hub.appRunning && hub.appQuitsAfter >= 0 {
		if hub.appQuitsAfter == 0 {
			hub.appRunning = false
		}
		hub.appQuitsAfter--
	}
	return hub.appRunning, nil
}

func (hub *fakeHub) QuitApp(context.Context) error {
	hub.record("quit app")
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hub.appRunning = false
	return nil
}

func (hub *fakeHub) OpenApp(_ context.Context, app string) error {
	version, err := BundleVersion(app)
	if err != nil {
		return err
	}
	hub.record("open app " + version)
	hub.mutex.Lock()
	hub.appRunning, hub.appQuitsAfter = true, -1
	opens := hub.appOpens
	hub.mutex.Unlock()
	if opens {
		go func() {
			time.Sleep(20 * time.Millisecond)
			_ = os.WriteFile(filepath.Join(hub.updates, LaunchedMark), []byte(version+"\n"), 0o600)
		}()
	}
	return nil
}

func (hub *fakeHub) ShowSteps(_ context.Context, app, statePath string) error {
	if statePath != hub.statePath() {
		hub.t.Errorf("steps window reads %s", statePath)
	}
	hub.record("show steps")
	return nil
}

func (hub *fakeHub) StopServer(context.Context) error {
	hub.record("stop server")
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if hub.stopFails {
		return errors.New("launchctl: the server is still running")
	}
	hub.answering = ""
	return nil
}

func (hub *fakeHub) StartServer(context.Context) error {
	version, err := BundleVersion(hub.installed())
	if err != nil {
		return err
	}
	hub.record("start server " + version)
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if hub.migrating[version] {
		hub.logLines = append(hub.logLines, `{"msg":"migrating the database","from":88}`)
		dump := filepath.Join(hub.backups, "hub-pre-migration-88.dump")
		if err := os.WriteFile(dump, []byte("dump"), 0o600); err != nil {
			return err
		}
		hub.logLines = append(hub.logLines, `{"msg":"database dumped before migrating"}`)
	}
	switch {
	case hub.crashing[version]:
		hub.logLines = append(hub.logLines, `{"level":"ERROR","msg":"hub-server stopped","error":"apply migrations: ERROR: column \"x\" does not exist"}`)
	case hub.failing[version]:
	case hub.slow[version]:
		go func() {
			for range 5 {
				time.Sleep(150 * time.Millisecond)
				hub.mutex.Lock()
				hub.logLines = append(hub.logLines, `{"msg":"migration applied","version":91}`)
				hub.mutex.Unlock()
			}
			hub.mutex.Lock()
			hub.answering = version
			hub.mutex.Unlock()
		}()
	default:
		hub.answering = version
	}
	return nil
}

func (hub *fakeHub) ServerLogEnd() int64 {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	return int64(len(hub.logLines))
}

func (hub *fakeHub) ReadServerLog(offset int64) ([]string, int64) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if offset > int64(len(hub.logLines)) {
		offset = int64(len(hub.logLines))
	}
	return slices.Clone(hub.logLines[offset:]), int64(len(hub.logLines))
}

func (hub *fakeHub) CountWrites(_ context.Context, hubServer string, migration int64) (WritesSinceMigration, error) {
	hub.record(fmt.Sprintf("count writes %s %d", hubServer, migration))
	dump := filepath.Join(hub.backups, fmt.Sprintf("hub-pre-migration-%d.dump", migration))
	info, err := os.Stat(dump)
	if err != nil {
		return WritesSinceMigration{}, nil
	}
	dumpedAt := info.ModTime()
	return WritesSinceMigration{Dump: dump, DumpedAt: &dumpedAt, Marked: hub.marked, Lost: hub.lost}, nil
}

func (hub *fakeHub) RestoreDatabase(_ context.Context, hubServer, dump string) error {
	hub.record("restore " + hubServer + " " + dump)
	if hub.restoreFails {
		return errors.New("pg_restore: the dump is damaged")
	}
	return nil
}

func (hub *fakeHub) PostUpdate(_ context.Context, title, body string) error {
	hub.record("post update " + title + " " + body)
	return nil
}

// requireInstalled checks a finished install: the new version in place, the
// old one kept, the install recorded and the job gone.
func (hub *fakeHub) requireInstalled(state State) {
	hub.t.Helper()
	if state.Step != StepInstalled {
		hub.t.Fatalf("ended at %s (%s %s), want installed; calls %v", state.Step, state.Failure, state.Error, hub.calls)
	}
	if !isVersion(hub.installed(), newVersion) || !isVersion(hub.previous(), oldVersion) {
		hub.t.Fatal("the new version isn't installed, or the old one isn't kept in previous/")
	}
	if _, err := os.Stat(filepath.Dir(hub.download())); !errors.Is(err, os.ErrNotExist) {
		hub.t.Fatalf("the download's folder is still there: %v", err)
	}
	if hub.answering != newVersion {
		hub.t.Fatalf("the server answers %q", hub.answering)
	}
	var installs []Install
	data, _ := os.ReadFile(filepath.Join(hub.updates, "installs.json"))
	if json.Unmarshal(data, &installs) != nil || len(installs) != 1 || installs[0].From != oldVersion || installs[0].To != newVersion {
		hub.t.Fatalf("installs.json = %s", data)
	}
	if _, err := os.Stat(hub.jobPlist()); !errors.Is(err, os.ErrNotExist) {
		hub.t.Fatal("the launchd job's plist is still there")
	}
}

// requireRolledBack checks a finished rollback: the old version in place
// and answering, the new one marked bad and gone, the feed told.
func (hub *fakeHub) requireRolledBack(state State, failure, lost string) {
	hub.t.Helper()
	if state.Step != StepRolledBack {
		hub.t.Fatalf("ended at %s (%s), want rolled back; calls %v", state.Step, state.Error, hub.calls)
	}
	if state.Failure != failure || state.Lost != lost {
		hub.t.Fatalf("failure %q, lost %q; want %q, %q", state.Failure, state.Lost, failure, lost)
	}
	if !isVersion(hub.installed(), oldVersion) || hub.answering != oldVersion {
		hub.t.Fatalf("installed %s answering %q, want the old version back", hub.installed(), hub.answering)
	}
	for _, app := range []string{hub.download(), hub.previous()} {
		if isVersion(app, newVersion) {
			hub.t.Fatalf("the failed version is still at %s", app)
		}
	}
	data, _ := os.ReadFile(filepath.Join(hub.updates, BadVersionsFile))
	if string(data) == "" || !strings.Contains(string(data), newVersion) {
		hub.t.Fatalf("%s isn't marked bad: %s", newVersion, data)
	}
	if hub.called("post update "+failure+" "+lost) != 1 {
		hub.t.Fatalf("the feed wasn't told: %v", hub.calls)
	}
}

func TestAnInstallStopsSwapsStartsChecksAndReopensTheApp(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(false, true)
	hub.appQuitsAfter = 2

	state := hub.run()
	hub.requireInstalled(state)
	if hub.called("show steps") != 1 || hub.called("open app "+newVersion) != 1 || hub.called("stop server") != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
	if hub.called("count writes") != 0 || hub.called("restore") != 0 {
		t.Fatalf("an install that worked counted or restored: %v", hub.calls)
	}
	if log, _ := os.ReadFile(filepath.Join(hub.updates, "install.log")); !strings.Contains(string(log), "Installed "+newVersion) {
		t.Fatalf("install.log = %s", log)
	}
}

func TestInstallWhenIQuitLeavesTheAppClosed(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(false, false)

	hub.requireInstalled(hub.run())
	if hub.called("open app") != 0 {
		t.Fatalf("the app opened: %v", hub.calls)
	}
}

func TestAnInstallWithMigrationsRecordsTheDumpAndWaitsForThemToFinish(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(true, true)
	// Its migrations take longer than the server's wait; their progress
	// moves the wait on.
	hub.slow[newVersion] = true

	state := hub.run()
	hub.requireInstalled(state)
	var installs []Install
	data, _ := os.ReadFile(filepath.Join(hub.updates, "installs.json"))
	_ = json.Unmarshal(data, &installs)
	if want := filepath.Join(hub.backups, "hub-pre-migration-88.dump"); installs[0].Dump != want {
		t.Fatalf("recorded dump %q, want %q", installs[0].Dump, want)
	}
	if state.Migrating {
		t.Fatal("still says it's migrating")
	}
}

func TestAServerThatFailsItsCheckRollsBackAndLosesNothing(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(false, true)
	hub.failing[newVersion] = true

	state := hub.run()
	hub.requireRolledBack(state, "0.1.252 couldn't start, so the hub went back to 0.1.247.", "Nothing was lost.")
	if hub.called("restore") != 0 || hub.called("count writes") != 0 {
		t.Fatalf("a version without migrations restored or counted: %v", hub.calls)
	}
	// The old app reopens, as the install closed it.
	if hub.called("open app "+oldVersion) != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
}

func TestAMigrationThatFailsRestoresTheDumpWithTheOldServer(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(true, true)
	hub.crashing[newVersion] = true

	start := time.Now()
	state := hub.run()
	hub.requireRolledBack(state, "0.1.252 couldn't start, so the hub went back to 0.1.247.", "Nothing was lost.")
	dump := filepath.Join(hub.backups, "hub-pre-migration-88.dump")
	if state.Dump != dump {
		t.Fatalf("dump = %q", state.Dump)
	}
	// Counted with the new version's server, restored with the old one's,
	// which the swap put where the download was.
	if hub.called(fmt.Sprintf("count writes %s 88", CommandPath(hub.installed(), "hub-server"))) != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
	if hub.called("restore "+CommandPath(hub.previous(), "hub-server")+" "+dump) != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
	// The server's log said it stopped: no waiting out the check.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the rollback took %v", elapsed)
	}
}

func TestARollbackAfterTheMarkSaysWhatItLost(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(true, true)
	hub.marked, hub.lost = true, "Lost: 2 updates."
	// It migrated and served a little, then the app it ships didn't open.
	hub.appOpens = false

	state := hub.run()
	hub.requireRolledBack(state, "Job Search Hub 0.1.252 didn't open, so the hub went back to 0.1.247.", "Lost: 2 updates.")
	if hub.called("quit app") != 1 || hub.called("restore") != 1 {
		t.Fatalf("calls = %v", hub.calls)
	}
}

func TestADumpFromAnEarlierInstallIsntRestored(t *testing.T) {
	hub := newFakeHub(t)
	state := hub.prepare(true, false)
	hub.migrating[newVersion] = false
	hub.failing[newVersion] = true
	old := filepath.Join(hub.backups, "hub-pre-migration-88.dump")
	if err := os.WriteFile(old, []byte("dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	long := state.StartedAt.Add(-time.Hour)
	if err := os.Chtimes(old, long, long); err != nil {
		t.Fatal(err)
	}

	state = hub.run()
	hub.requireRolledBack(state, "0.1.252 couldn't start, so the hub went back to 0.1.247.", "Nothing was lost.")
	if state.Dump != "" || hub.called("restore") != 0 {
		t.Fatalf("restored an earlier install's dump: %v", hub.calls)
	}
}

func TestAnAppThatDoesntQuitAbandonsTheInstall(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(false, true)
	hub.appQuitsAfter = -1

	state := hub.run()
	if state.Step != StepAbandoned || state.Failure != "Job Search Hub didn't quit, so nothing was installed." {
		t.Fatalf("state = %+v", state)
	}
	if !isVersion(hub.installed(), oldVersion) || hub.called("stop server") != 0 {
		t.Fatalf("something changed: %v", hub.calls)
	}
}

func TestAServerThatDoesntStopAbandonsTheInstall(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(false, true)
	hub.stopFails = true

	state := hub.run()
	if state.Step != StepAbandoned || !isVersion(hub.installed(), oldVersion) || !isVersion(hub.download(), newVersion) {
		t.Fatalf("state = %+v", state)
	}
	if hub.called("open app "+oldVersion) != 1 {
		t.Fatalf("the app wasn't reopened: %v", hub.calls)
	}
}

func TestARollbackThatFailsStopsAndSaysHowToFinish(t *testing.T) {
	hub := newFakeHub(t)
	hub.prepare(true, true)
	hub.crashing[newVersion] = true
	hub.restoreFails = true

	state := hub.run()
	if state.Step != StepRollbackFailed {
		t.Fatalf("ended at %s", state.Step)
	}
	if !strings.Contains(state.Error, "restoring_database") || !strings.Contains(state.Error, "the dump is damaged") {
		t.Fatalf("error = %q", state.Error)
	}
	dump := filepath.Join(hub.backups, "hub-pre-migration-88.dump")
	want := []string{
		"launchctl kill SIGTERM gui/501/com.tonypine.jobsearchhub.server",
		quote(CommandPath(hub.previous(), "hub-server")) + " database restore " + quote(dump),
		"mv " + quote(hub.installed()) + " " + quote(filepath.Join(hub.updates, "failed-0.1.252.app")) + " && mv " + quote(hub.previous()) + " " + quote(hub.installed()),
		"launchctl kickstart gui/501/com.tonypine.jobsearchhub.server",
	}
	if !slices.Equal(state.Commands, want) {
		t.Fatalf("commands =\n%s\nwant\n%s", strings.Join(state.Commands, "\n"), strings.Join(want, "\n"))
	}
	// It leaves things as they are: the new version stays installed.
	if !isVersion(hub.installed(), newVersion) {
		t.Fatal("the rollback went on after it failed")
	}
	// A run after that does nothing more.
	if again := hub.run(); again.Step != StepRollbackFailed || hub.called("restore") != 1 {
		t.Fatalf("a second run went on: %v", hub.calls)
	}
}

func TestAnInstallThatKeepsCrashingGivesUp(t *testing.T) {
	hub := newFakeHub(t)
	state := hub.prepare(false, false)
	state.Step, state.Runs = StepCheckingServer, DefaultMaxRuns
	hub.save(state)

	state = hub.run()
	if state.Step != StepRollbackFailed || len(state.Commands) == 0 {
		t.Fatalf("state = %+v", state)
	}
}

func TestARollbackThatStopsBeforeCountingRestoresTheDump(t *testing.T) {
	for _, stop := range []string{"the new server doesn't stop", "hub-update gives up"} {
		t.Run(stop, func(t *testing.T) {
			hub := newFakeHub(t)
			state := hub.prepare(true, false)
			dump := filepath.Join(hub.backups, "hub-pre-migration-88.dump")
			if err := os.WriteFile(dump, []byte("dump"), 0o600); err != nil {
				t.Fatal(err)
			}
			if stop == "hub-update gives up" {
				// The new server migrated, then crashed each time.
				hub.layOut(StepCheckingServer, true, true)
				state.Step, state.Runs = StepCheckingServer, DefaultMaxRuns
			} else {
				hub.layOut(StepStoppingNewServer, true, true)
				hub.stopFails = true
				state.Step, state.Runs = StepStoppingNewServer, 1
			}
			hub.save(state)

			state = hub.run()
			if state.Step != StepRollbackFailed {
				t.Fatalf("ended at %s", state.Step)
			}
			restore := quote(CommandPath(hub.previous(), "hub-server")) + " database restore " + quote(dump)
			if !slices.Contains(state.Commands, restore) || state.Dump != dump {
				t.Fatalf("commands =\n%s\nwant the restore of %s", strings.Join(state.Commands, "\n"), dump)
			}
		})
	}
}

func TestARunCutShortWhileCheckingTheServerResumesThere(t *testing.T) {
	hub := newFakeHub(t)
	state := hub.prepare(true, true)
	hub.layOut(StepCheckingServer, true, true)
	// The new server is still migrating when hub-update is told to stop.
	hub.answering = ""
	state.Step, state.Runs = StepCheckingServer, 1
	hub.save(state)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := hub.machine().Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	saved, err := LoadState(hub.statePath())
	if err != nil || saved.Step != StepCheckingServer || saved.Failure != "" {
		t.Fatalf("saved state = %+v, %v", saved, err)
	}
	if hub.called("stop server") != 0 {
		t.Fatalf("the cut-short run rolled back: %v", hub.calls)
	}

	hub.answering = newVersion
	hub.requireInstalled(hub.run())
}

// layOut puts the bundles where the install leaves them just before step,
// with the server as it would be.
func (hub *fakeHub) layOut(step Step, swapped, kept bool) {
	hub.t.Helper()
	if swapped {
		if err := swap(hub.download(), hub.installed()); err != nil {
			hub.t.Fatal(err)
		}
	}
	if kept {
		if err := os.MkdirAll(filepath.Dir(hub.previous()), 0o700); err != nil {
			hub.t.Fatal(err)
		}
		if err := os.Rename(hub.download(), hub.previous()); err != nil {
			hub.t.Fatal(err)
		}
	}
	hub.appRunning = false
	switch step {
	case StepWaitingForApp, StepStoppingServer:
		hub.answering = oldVersion
	case StepCheckingServer, StepOpeningApp, StepRecording:
		hub.answering = newVersion
	case StepCheckingApp:
		hub.answering = newVersion
		hub.appRunning, hub.appQuitsAfter = true, -1
		_ = os.WriteFile(filepath.Join(hub.updates, LaunchedMark), []byte(newVersion), 0o600)
	default:
		hub.answering = ""
	}
}

func TestAnInstallResumesFromEachStep(t *testing.T) {
	for _, resume := range []struct {
		step          Step
		swapped, kept bool
	}{
		{StepWaitingForApp, false, false},
		{StepStoppingServer, false, false},
		{StepSwapping, false, false},
		// Swapped, but stopped before the old version moved to previous/.
		{StepSwapping, true, false},
		{StepSwapping, true, true},
		{StepStartingServer, true, true},
		{StepCheckingServer, true, true},
		{StepOpeningApp, true, true},
		{StepCheckingApp, true, true},
		{StepRecording, true, true},
	} {
		t.Run(fmt.Sprintf("%s swapped %v kept %v", resume.step, resume.swapped, resume.kept), func(t *testing.T) {
			hub := newFakeHub(t)
			state := hub.prepare(false, true)
			hub.layOut(resume.step, resume.swapped, resume.kept)
			state.Step, state.Runs = resume.step, 1
			hub.save(state)

			state = hub.run()
			hub.requireInstalled(state)
			if state.Runs != 2 {
				t.Fatalf("runs = %d", state.Runs)
			}
		})
	}
}

func TestARollbackResumesFromEachStep(t *testing.T) {
	for _, resume := range []struct {
		step          Step
		swapped, kept bool
	}{
		{StepStoppingNewServer, true, true},
		{StepCountingWrites, true, true},
		{StepRestoringDatabase, true, true},
		{StepSwappingBack, true, true},
		// Swapped back, but stopped before the new version was deleted.
		{StepSwappingBack, false, false},
		{StepStartingOldServer, false, false},
		{StepCheckingOldServer, false, false},
		{StepReporting, false, false},
	} {
		t.Run(fmt.Sprintf("%s swapped %v kept %v", resume.step, resume.swapped, resume.kept), func(t *testing.T) {
			hub := newFakeHub(t)
			state := hub.prepare(true, false)
			if err := os.WriteFile(filepath.Join(hub.backups, "hub-pre-migration-88.dump"), []byte("dump"), 0o600); err != nil {
				t.Fatal(err)
			}
			hub.layOut(resume.step, resume.swapped, resume.kept)
			switch resume.step {
			case StepCheckingOldServer, StepReporting:
				hub.answering = oldVersion
				fallthrough
			case StepStartingOldServer:
				// Swapping back deleted the new version.
				if err := os.RemoveAll(hub.download()); err != nil {
					t.Fatal(err)
				}
			}
			state.Step, state.Runs = resume.step, 1
			state.Failure = "0.1.252 couldn't start, so the hub went back to 0.1.247."
			if resume.step != StepStoppingNewServer && resume.step != StepCountingWrites {
				state.Dump, state.Lost = filepath.Join(hub.backups, "hub-pre-migration-88.dump"), "Nothing was lost."
			}
			hub.save(state)

			state = hub.run()
			hub.requireRolledBack(state, "0.1.252 couldn't start, so the hub went back to 0.1.247.", "Nothing was lost.")
			restores := 0
			if resume.step == StepStoppingNewServer || resume.step == StepCountingWrites || resume.step == StepRestoringDatabase {
				restores = 1
			}
			if hub.called("restore") != restores {
				t.Fatalf("restores = %d, want %d: %v", hub.called("restore"), restores, hub.calls)
			}
		})
	}
}

func TestAStateWithoutItsFieldsIsRefused(t *testing.T) {
	hub := newFakeHub(t)
	hub.save(State{From: oldVersion, Step: StepWaitingForApp})
	if _, err := hub.machine().Run(context.Background()); err == nil || !strings.Contains(err.Error(), "[installed new_app to]") {
		t.Fatalf("err = %v", err)
	}
}
