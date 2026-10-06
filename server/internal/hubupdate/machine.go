package hubupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// System is what an install does outside its folders: the app's process,
// the server's launchd agent, and the commands in the bundles. The tests
// fake it, against a fake server.
type System interface {
	IsAppRunning(ctx context.Context) (bool, error)
	// QuitApp ends the app, as when a new one hangs at launch.
	QuitApp(ctx context.Context) error
	OpenApp(ctx context.Context, app string) error
	// ShowSteps starts the steps window from app, which reads statePath
	// until the install ends or the app opens.
	ShowSteps(ctx context.Context, app, statePath string) error
	// StopServer sends the server SIGTERM and waits for it to exit, which
	// it does cleanly, so launchd leaves it stopped. A stopped server is no
	// error.
	StopServer(ctx context.Context) error
	// StartServer has launchd start the server, from whichever bundle is
	// installed.
	StartServer(ctx context.Context) error
	// ServerLogEnd is the size of the server's log, and ReadServerLog its
	// whole lines from offset on, with the offset after them.
	ServerLogEnd() int64
	ReadServerLog(offset int64) ([]string, int64)
	// CountWrites runs hubServer database count-writes <migration>.
	CountWrites(ctx context.Context, hubServer string, migration int64) (WritesSinceMigration, error)
	// RestoreDatabase runs hubServer database restore <dump>.
	RestoreDatabase(ctx context.Context, hubServer, dump string) error
	// PostUpdate records an update in the hub's feed, which the phone hears.
	PostUpdate(ctx context.Context, title, body string) error
}

// WritesSinceMigration is what hub-server database count-writes prints.
type WritesSinceMigration struct {
	Dump     string     `json:"dump,omitempty"`
	DumpedAt *time.Time `json:"dumped_at,omitempty"`
	Marked   bool       `json:"marked"`
	Lost     string     `json:"lost,omitempty"`
}

// Machine takes an install's steps, from the one its state records.
type Machine struct {
	StatePath string
	// Updates is ~/Library/Application Support/JobSearchHub/Updates: the
	// previous version, the versions marked bad, the installs, the log, and
	// the app's launched mark.
	Updates string
	// Backups is where the server keeps its dumps.
	Backups string
	// HubURL reaches the server, and Service names its launchd agent in
	// the commands a failed rollback leaves.
	HubURL  string
	Service string
	System  System
	Client  *http.Client
	Now     func() time.Time
	// Poll is how often waits look again.
	Poll time.Duration
	// AppQuitTimeout is how long the app gets to quit; ServerTimeout and
	// AppTimeout how long the new server and app get to pass their checks;
	// MigrationGrace how long the server gets after each sign of progress
	// in its migrations.
	AppQuitTimeout time.Duration
	ServerTimeout  time.Duration
	AppTimeout     time.Duration
	MigrationGrace time.Duration
	// MaxRuns is how many times hub-update runs one install before it
	// gives up on it.
	MaxRuns int

	stepsShown bool
}

// Defaults for the timeouts and the runs.
const (
	DefaultPoll           = 500 * time.Millisecond
	DefaultAppQuitTimeout = 5 * time.Minute
	DefaultServerTimeout  = 60 * time.Second
	DefaultAppTimeout     = 60 * time.Second
	DefaultMigrationGrace = 10 * time.Minute
	DefaultMaxRuns        = 5
)

// LaunchedMark is the file in Updates the app writes its version to once
// its window is up and connected.
const LaunchedMark = "launched"

// Log lines of the server that say where its start is.
const (
	logMigrating = "migrating the database"
	logDumped    = "database dumped before migrating"
	logMigrated  = "migration applied"
	logStopped   = "hub-server stopped"
)

// errStateLost stops a run that can't record its steps.
var errStateLost = errors.New("the install's state can't be written")

// Run takes the install's steps until it ends, and returns how it ended.
// An error means this run stopped part way, with the state saved; the
// next run picks it up.
func (machine *Machine) Run(ctx context.Context) (State, error) {
	state, err := LoadState(machine.StatePath)
	if err != nil {
		return State{}, err
	}
	if err := state.validate(); err != nil {
		return state, err
	}
	if state.Step.IsFinished() {
		return state, nil
	}
	state.Runs++
	machine.log("hub-update run %d of the install of %s over %s, at %s", state.Runs, state.To, state.From, state.Step)
	if state.Runs > machine.MaxRuns {
		machine.failRollback(ctx, &state, fmt.Errorf("hub-update stopped %d times on this install", state.Runs-1))
	}
	if err := machine.save(state); err != nil {
		return state, err
	}
	for !state.Step.IsFinished() {
		if err := ctx.Err(); err != nil {
			return state, err
		}
		if state.Step != StepWaitingForApp {
			machine.showSteps(ctx, state)
		}
		machine.take(ctx, &state)
		if err := machine.save(state); err != nil {
			return state, err
		}
	}
	machine.finish(state)
	return state, nil
}

// take takes the state's step, which moves it to the next.
func (machine *Machine) take(ctx context.Context, state *State) {
	switch state.Step {
	case StepWaitingForApp:
		machine.waitForApp(ctx, state)
	case StepStoppingServer:
		machine.stopServer(ctx, state)
	case StepSwapping:
		machine.swapBundles(ctx, state)
	case StepStartingServer:
		machine.startServer(ctx, state)
	case StepCheckingServer:
		machine.checkServer(ctx, state)
	case StepOpeningApp:
		machine.openApp(ctx, state)
	case StepCheckingApp:
		machine.checkApp(ctx, state)
	case StepRecording:
		machine.record(state)
	case StepStoppingNewServer:
		machine.stopNewServer(ctx, state)
	case StepCountingWrites:
		machine.countWrites(ctx, state)
	case StepRestoringDatabase:
		machine.restoreDatabase(ctx, state)
	case StepSwappingBack:
		machine.swapBack(ctx, state)
	case StepStartingOldServer:
		if err := machine.System.StartServer(ctx); err != nil {
			if cutShort(ctx) {
				return
			}
			machine.failRollback(ctx, state, fmt.Errorf("start %s's server: %w", state.From, err))
			return
		}
		state.Step = StepCheckingOldServer
	case StepCheckingOldServer:
		if err := machine.waitForServer(ctx, state, state.From, false); err != nil {
			if cutShort(ctx) {
				return
			}
			machine.failRollback(ctx, state, fmt.Errorf("%s's server didn't come back: %w", state.From, err))
			return
		}
		state.Step = StepReporting
	case StepReporting:
		machine.report(ctx, state)
	default:
		machine.failRollback(ctx, state, fmt.Errorf("the install is at a step this hub-update doesn't know, %q", state.Step))
	}
}

// waitForApp waits for the app to quit, which it does once it started this
// install.
func (machine *Machine) waitForApp(ctx context.Context, state *State) {
	deadline := machine.Now().Add(machine.AppQuitTimeout)
	for {
		running, err := machine.System.IsAppRunning(ctx)
		if err == nil && !running {
			machine.log("The app has quit.")
			state.Step = StepStoppingServer
			return
		}
		if machine.Now().After(deadline) {
			machine.abandon(ctx, state, "Job Search Hub didn't quit, so nothing was installed.")
			return
		}
		if !machine.sleep(ctx) {
			return
		}
	}
}

func (machine *Machine) stopServer(ctx context.Context, state *State) {
	machine.log("Stopping the server.")
	if err := machine.System.StopServer(ctx); err != nil {
		if cutShort(ctx) {
			return
		}
		machine.log("The server didn't stop: %v", err)
		machine.abandon(ctx, state, "The server didn't stop, so nothing was installed.")
		return
	}
	state.Step = StepSwapping
}

// swapBundles puts the new bundle in place and the old one in previous/.
// Each half can be taken again: the bundles' versions say what's done.
func (machine *Machine) swapBundles(ctx context.Context, state *State) {
	if !isVersion(state.Installed, state.To) {
		if !isVersion(state.NewApp, state.To) || !isVersion(state.Installed, state.From) {
			machine.log("Neither %s nor %s is where the install left it.", state.NewApp, state.Installed)
			machine.abandon(ctx, state, fmt.Sprintf("%s wasn't where the install left it, so nothing was installed.", state.To))
			return
		}
		if err := swap(state.NewApp, state.Installed); err != nil {
			machine.log("The swap failed: %v", err)
			machine.abandon(ctx, state, fmt.Sprintf("%s couldn't be put in place, so nothing was installed.", state.To))
			return
		}
		machine.log("%s is in place at %s.", state.To, state.Installed)
	}
	if isVersion(state.NewApp, state.From) {
		previous := filepath.Join(machine.Updates, "previous")
		if err := os.RemoveAll(previous); err == nil {
			err = os.MkdirAll(previous, 0o700)
			if err == nil {
				err = os.Rename(state.NewApp, filepath.Join(previous, filepath.Base(state.Installed)))
			}
			if err != nil {
				// The old version stays where the swap put it, which a
				// rollback finds too.
				machine.log("Couldn't keep %s in previous/: %v", state.From, err)
			}
		}
	}
	state.Step = StepStartingServer
}

func (machine *Machine) startServer(ctx context.Context, state *State) {
	state.ServerLog = machine.System.ServerLogEnd()
	machine.log("Starting %s's server.", state.To)
	if err := machine.System.StartServer(ctx); err != nil {
		if cutShort(ctx) {
			return
		}
		machine.rollBack(state, fmt.Sprintf("%s couldn't start, so the hub went back to %s.", state.To, state.From), err)
		return
	}
	state.Step = StepCheckingServer
}

func (machine *Machine) checkServer(ctx context.Context, state *State) {
	if err := machine.waitForServer(ctx, state, state.To, true); err != nil {
		if cutShort(ctx) {
			return
		}
		machine.rollBack(state, fmt.Sprintf("%s couldn't start, so the hub went back to %s.", state.To, state.From), err)
		return
	}
	state.Migrating = false
	machine.log("%s's server is up.", state.To)
	if state.ReopenApp {
		state.Step = StepOpeningApp
	} else {
		state.Step = StepRecording
	}
}

// waitForServer waits for the server to answer GET /v1/version with
// version and GET /v1/health with 200. With readLog, its log moves the
// deadline on while migrations make progress, and ends the wait when the
// server stops.
func (machine *Machine) waitForServer(ctx context.Context, state *State, version string, readLog bool) error {
	deadline := machine.Now().Add(machine.ServerTimeout)
	offset := state.ServerLog
	for {
		if readLog {
			lines, next := machine.System.ReadServerLog(offset)
			offset = next
			for _, line := range lines {
				var entry struct {
					Message string `json:"msg"`
					Error   string `json:"error"`
				}
				if json.Unmarshal([]byte(line), &entry) != nil {
					continue
				}
				switch entry.Message {
				case logMigrating, logDumped, logMigrated:
					if !state.Migrating {
						state.Migrating = true
						machine.log("The new server is updating the database.")
						_ = machine.save(*state)
					}
					deadline = maxTime(deadline, machine.Now().Add(machine.MigrationGrace))
				case logStopped:
					return fmt.Errorf("the server stopped: %s", entry.Error)
				}
			}
		}
		answered, err := machine.readVersion(ctx)
		if err == nil && answered == version && machine.isHealthy(ctx) {
			return nil
		}
		if machine.Now().After(deadline) {
			if err == nil {
				err = fmt.Errorf("it answered %q", answered)
			}
			return fmt.Errorf("no healthy server on %s within the wait: %w", version, err)
		}
		if !machine.sleep(ctx) {
			return ctx.Err()
		}
	}
}

func (machine *Machine) openApp(ctx context.Context, state *State) {
	_ = os.Remove(filepath.Join(machine.Updates, LaunchedMark))
	if err := machine.System.OpenApp(ctx, state.Installed); err != nil {
		if cutShort(ctx) {
			return
		}
		machine.rollBack(state, fmt.Sprintf("Job Search Hub %s didn't open, so the hub went back to %s.", state.To, state.From), err)
		return
	}
	state.Step = StepCheckingApp
}

// checkApp waits for the app's launched mark, which it writes once its
// window is up and connected.
func (machine *Machine) checkApp(ctx context.Context, state *State) {
	failure := fmt.Sprintf("Job Search Hub %s didn't open, so the hub went back to %s.", state.To, state.From)
	deadline := machine.Now().Add(machine.AppTimeout)
	seenRunning := false
	for {
		if mark, err := os.ReadFile(filepath.Join(machine.Updates, LaunchedMark)); err == nil && strings.TrimSpace(string(mark)) == state.To {
			machine.log("Job Search Hub %s opened.", state.To)
			state.Step = StepRecording
			return
		}
		running, err := machine.System.IsAppRunning(ctx)
		if err == nil && running {
			seenRunning = true
		} else if err == nil && seenRunning {
			machine.rollBack(state, failure, errors.New("the app quit before it said it had opened"))
			return
		}
		if machine.Now().After(deadline) {
			machine.rollBack(state, failure, errors.New("the app didn't say it had opened within the wait"))
			return
		}
		if !machine.sleep(ctx) {
			return
		}
	}
}

// Install is one install, as Updates/installs.json records it, which going
// back reads.
type Install struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	Dump string    `json:"dump,omitempty"`
	At   time.Time `json:"at"`
}

func (machine *Machine) record(state *State) {
	install := Install{From: state.From, To: state.To, At: machine.Now()}
	if dump, ok := machine.findFreshDump(*state); ok {
		install.Dump = dump
	}
	path := filepath.Join(machine.Updates, "installs.json")
	var installs []Install
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &installs)
	}
	installs = append(installs, install)
	if err := writeJSON(path, installs); err != nil {
		machine.log("Couldn't record the install: %v", err)
	}
	// The download's folder is empty but for its checks.
	if folder := filepath.Dir(state.NewApp); filepath.Base(folder) == state.To && filepath.Dir(folder) == machine.Updates {
		_ = os.RemoveAll(folder)
	}
	machine.log("Installed %s.", state.To)
	state.Step = StepInstalled
}

// rollBack starts the rollback, saying why in the owner's words, and why in
// the log.
func (machine *Machine) rollBack(state *State, failure string, cause error) {
	machine.log("Rolling back: %s (%v)", failure, cause)
	state.Failure = failure
	state.Migrating = false
	state.Step = StepStoppingNewServer
}

func (machine *Machine) stopNewServer(ctx context.Context, state *State) {
	if running, err := machine.System.IsAppRunning(ctx); err == nil && running && isVersion(state.Installed, state.To) {
		if err := machine.System.QuitApp(ctx); err != nil {
			if cutShort(ctx) {
				return
			}
			machine.failRollback(ctx, state, fmt.Errorf("quit %s's app: %w", state.To, err))
			return
		}
	}
	if err := machine.System.StopServer(ctx); err != nil {
		if cutShort(ctx) {
			return
		}
		machine.failRollback(ctx, state, fmt.Errorf("stop %s's server: %w", state.To, err))
		return
	}
	state.Step = StepCountingWrites
}

// countWrites finds the dump the new server took before migrating, and
// what it wrote since, which restoring the dump loses. Without a dump no
// migration ran, and nothing is lost.
func (machine *Machine) countWrites(ctx context.Context, state *State) {
	state.Step = StepRestoringDatabase
	state.Lost = "Nothing was lost."
	if state.ToMigration <= state.FromMigration {
		return
	}
	newApp, found := machine.findBundle(*state, state.To)
	if found {
		counted, err := machine.System.CountWrites(ctx, CommandPath(newApp, "hub-server"), state.FromMigration)
		if err == nil {
			if counted.Dump != "" && counted.DumpedAt != nil && !counted.DumpedAt.Before(state.StartedAt) {
				state.Dump = counted.Dump
				if counted.Lost != "" {
					state.Lost = counted.Lost
				}
			}
			machine.log("Counted what %s wrote: %s", state.To, state.Lost)
			return
		}
		if cutShort(ctx) {
			state.Step = StepCountingWrites
			return
		}
		machine.log("Couldn't count what %s wrote: %v", state.To, err)
	}
	if dump, ok := machine.findFreshDump(*state); ok {
		state.Dump = dump
		state.Lost = fmt.Sprintf("Anything %s wrote couldn't be counted, and was lost.", state.To)
	}
}

func (machine *Machine) restoreDatabase(ctx context.Context, state *State) {
	if state.Dump != "" {
		oldApp, found := machine.findBundle(*state, state.From)
		if !found {
			machine.failRollback(ctx, state, fmt.Errorf("%s isn't on this Mac any more, to restore the database with", state.From))
			return
		}
		machine.log("Restoring %s.", state.Dump)
		if err := machine.System.RestoreDatabase(ctx, CommandPath(oldApp, "hub-server"), state.Dump); err != nil {
			if cutShort(ctx) {
				return
			}
			machine.failRollback(ctx, state, fmt.Errorf("restore %s: %w", state.Dump, err))
			return
		}
	}
	state.Step = StepSwappingBack
}

// swapBack puts the old bundle back in place, and deletes the new one.
func (machine *Machine) swapBack(ctx context.Context, state *State) {
	if !isVersion(state.Installed, state.From) {
		oldApp, found := machine.findBundle(*state, state.From)
		if !found || !isVersion(state.Installed, state.To) {
			machine.failRollback(ctx, state, fmt.Errorf("%s isn't on this Mac to go back to", state.From))
			return
		}
		if err := swap(oldApp, state.Installed); err != nil {
			machine.failRollback(ctx, state, fmt.Errorf("put %s back: %w", state.From, err))
			return
		}
		machine.log("%s is back in place.", state.From)
	}
	// The new version sits where the old one was; it isn't installed
	// again.
	for _, app := range []string{state.NewApp, filepath.Join(machine.Updates, "previous", filepath.Base(state.Installed))} {
		if isVersion(app, state.To) {
			_ = os.RemoveAll(app)
		}
	}
	state.Step = StepStartingOldServer
}

// report marks the version bad on this Mac, says what happened in the
// hub's feed, and reopens the old app if the install closed it.
func (machine *Machine) report(ctx context.Context, state *State) {
	if err := machine.markBad(state.To); err != nil {
		machine.log("Couldn't mark %s bad: %v", state.To, err)
	}
	if err := machine.System.PostUpdate(ctx, state.Failure, state.Lost); err != nil {
		if cutShort(ctx) {
			return
		}
		machine.log("Couldn't post the rollback to the feed: %v", err)
	}
	if state.ReopenApp {
		if err := machine.System.OpenApp(ctx, state.Installed); err != nil {
			machine.log("Couldn't reopen the app: %v", err)
		}
	}
	machine.log("Rolled back: %s %s", state.Failure, state.Lost)
	state.Step = StepRolledBack
}

// BadVersionsFile lists, in Updates, the versions that failed on this Mac,
// which the app doesn't offer again.
const BadVersionsFile = "bad-versions.json"

func (machine *Machine) markBad(version string) error {
	path := filepath.Join(machine.Updates, BadVersionsFile)
	var versions []string
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &versions)
	}
	if slices.Contains(versions, version) {
		return nil
	}
	return writeJSON(path, append(versions, version))
}

// abandon ends an install before anything changed: the server, if it
// stopped, starts again, and the app reopens.
func (machine *Machine) abandon(ctx context.Context, state *State, failure string) {
	machine.log("Abandoned: %s", failure)
	state.Failure = failure
	state.Step = StepAbandoned
	if err := machine.System.StartServer(ctx); err != nil {
		machine.log("Couldn't start the server again: %v", err)
	}
	if state.ReopenApp {
		if err := machine.System.OpenApp(ctx, state.Installed); err != nil {
			machine.log("Couldn't reopen the app: %v", err)
		}
	}
}

// failRollback stops where the rollback is, and says how to finish it.
func (machine *Machine) failRollback(ctx context.Context, state *State, err error) {
	if !state.Step.isRollback() {
		// Gave up before a rollback started: finish it from its start.
		state.Step = StepStoppingNewServer
	}
	if state.Dump == "" && (state.Step == StepStoppingNewServer || state.Step == StepCountingWrites) {
		// Counting writes, which finds the dump, is still to come: restore
		// the one the new server took, if it migrated.
		if dump, ok := machine.findFreshDump(*state); ok {
			state.Dump = dump
		}
	}
	machine.log("The rollback stopped at %s: %v", state.Step, err)
	state.Error = fmt.Sprintf("The rollback stopped at %s: %v", state.Step, err)
	state.Commands = machine.commandsToFinish(*state)
	state.Step = StepRollbackFailed
	if state.Failure == "" {
		state.Failure = fmt.Sprintf("%s couldn't be installed, and the hub couldn't go back to %s by itself.", state.To, state.From)
	}
}

// commandsToFinish are the commands that finish the rollback from its step,
// run in Terminal.
func (machine *Machine) commandsToFinish(state State) []string {
	order := []Step{StepStoppingNewServer, StepCountingWrites, StepRestoringDatabase, StepSwappingBack, StepStartingOldServer, StepCheckingOldServer}
	from := slices.Index(order, state.Step)
	if from < 0 {
		from = 0
	}
	remains := func(step Step) bool { return slices.Index(order, step) >= from }
	oldApp, found := machine.findBundle(state, state.From)
	if !found {
		oldApp = filepath.Join(machine.Updates, "previous", filepath.Base(state.Installed))
	}
	var commands []string
	if remains(StepStoppingNewServer) || remains(StepRestoringDatabase) || remains(StepSwappingBack) {
		commands = append(commands, "launchctl kill SIGTERM "+machine.Service)
	}
	if state.Dump != "" && remains(StepRestoringDatabase) {
		commands = append(commands, quote(CommandPath(oldApp, "hub-server"))+" database restore "+quote(state.Dump))
	}
	if remains(StepSwappingBack) && !isVersion(state.Installed, state.From) {
		failed := filepath.Join(machine.Updates, "failed-"+state.To+".app")
		commands = append(commands, "mv "+quote(state.Installed)+" "+quote(failed)+" && mv "+quote(oldApp)+" "+quote(state.Installed))
	}
	commands = append(commands, "launchctl kickstart "+machine.Service)
	return commands
}

// finish ends the run: the launchd job that runs hub-update goes.
func (machine *Machine) finish(state State) {
	if state.JobPlist != "" {
		_ = os.Remove(state.JobPlist)
	}
	machine.log("The install of %s ended: %s", state.To, state.Step)
}

// findBundle is the bundle of version among the places an install puts
// one: installed, downloaded, or kept in previous/.
func (machine *Machine) findBundle(state State, version string) (string, bool) {
	for _, app := range []string{state.Installed, state.NewApp, filepath.Join(machine.Updates, "previous", filepath.Base(state.Installed))} {
		if isVersion(app, version) {
			return app, true
		}
	}
	return "", false
}

// findFreshDump is the dump this install's server took before migrating,
// when there is one.
func (machine *Machine) findFreshDump(state State) (string, bool) {
	if machine.Backups == "" || state.ToMigration <= state.FromMigration {
		return "", false
	}
	dump := filepath.Join(machine.Backups, fmt.Sprintf("hub-pre-migration-%d.dump", state.FromMigration))
	info, err := os.Stat(dump)
	if err != nil || info.ModTime().Before(state.StartedAt) {
		return "", false
	}
	return dump, true
}

func (machine *Machine) showSteps(ctx context.Context, state State) {
	if machine.stepsShown {
		return
	}
	machine.stepsShown = true
	app, found := machine.findBundle(state, state.To)
	if !found {
		app, found = machine.findBundle(state, state.From)
	}
	if !found {
		return
	}
	if err := machine.System.ShowSteps(ctx, app, machine.StatePath); err != nil {
		machine.log("Couldn't show the steps: %v", err)
	}
}

func (machine *Machine) readVersion(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, machine.HubURL+"/v1/version", nil)
	if err != nil {
		return "", err
	}
	response, err := machine.Client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET /v1/version answered %d", response.StatusCode)
	}
	var build struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(response.Body).Decode(&build); err != nil {
		return "", err
	}
	return build.Version, nil
}

func (machine *Machine) isHealthy(ctx context.Context) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, machine.HubURL+"/v1/health", nil)
	if err != nil {
		return false
	}
	response, err := machine.Client.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func (machine *Machine) save(state State) error {
	if err := state.Save(machine.StatePath); err != nil {
		return fmt.Errorf("%w: %v", errStateLost, err)
	}
	return nil
}

// cutShort reports whether ctx ended, as when hub-update is told to stop.
// A step cut short leaves its step as it is, for the next run to take
// again, rather than count a failed call as the install's failure.
func cutShort(ctx context.Context) bool {
	return ctx.Err() != nil
}

// sleep waits one poll, and reports whether ctx is still live.
func (machine *Machine) sleep(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(machine.Poll):
		return true
	}
}

// log adds a line to Updates/install.log, which Settings › Version shows.
func (machine *Machine) log(format string, arguments ...any) {
	line := machine.Now().Format(time.RFC3339) + " " + fmt.Sprintf(format, arguments...) + "\n"
	file, err := os.OpenFile(filepath.Join(machine.Updates, "install.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	partial := path + ".partial"
	if err := os.WriteFile(partial, data, 0o600); err != nil {
		return err
	}
	return os.Rename(partial, path)
}

// quote single-quotes a path for a shell.
func quote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func maxTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
