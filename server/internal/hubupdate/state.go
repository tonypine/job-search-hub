// Package hubupdate installs a new version of the Mac app's bundle once the
// app has quit: it stops the server, swaps the bundle, starts the server
// and checks it, reopens the app and checks that too, and puts the old
// version back if either fails. Every step is recorded in
// Updates/state.json before it's taken, and every step can be taken again,
// so a run cut short by sleep, a power loss or a crash finishes or undoes
// the install when it runs again.
package hubupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Step is where an install is. The app reads it too, and the steps window
// shows it.
type Step string

const (
	// The install's steps, in order.
	StepWaitingForApp  Step = "waiting_for_app"
	StepStoppingServer Step = "stopping_server"
	StepSwapping       Step = "swapping"
	StepStartingServer Step = "starting_server"
	StepCheckingServer Step = "checking_server"
	StepOpeningApp     Step = "opening_app"
	StepCheckingApp    Step = "checking_app"
	StepRecording      Step = "recording"

	// A rollback's steps, in order.
	StepStoppingNewServer Step = "stopping_new_server"
	StepCountingWrites    Step = "counting_writes"
	StepRestoringDatabase Step = "restoring_database"
	StepSwappingBack      Step = "swapping_back"
	StepStartingOldServer Step = "starting_old_server"
	StepCheckingOldServer Step = "checking_old_server"
	StepReporting         Step = "reporting"

	// Where an install ends.
	StepInstalled Step = "installed"
	// Nothing changed: the app or the server didn't stop.
	StepAbandoned  Step = "abandoned"
	StepRolledBack Step = "rolled_back"
	// The rollback stopped part way; the state says where, and Commands
	// finish it.
	StepRollbackFailed Step = "rollback_failed"
)

// IsFinished reports whether the install has ended, one way or another.
func (step Step) IsFinished() bool {
	switch step {
	case StepInstalled, StepAbandoned, StepRolledBack, StepRollbackFailed:
		return true
	}
	return false
}

// isRollback reports whether the step belongs to a rollback.
func (step Step) isRollback() bool {
	switch step {
	case StepStoppingNewServer, StepCountingWrites, StepRestoringDatabase, StepSwappingBack, StepStartingOldServer, StepCheckingOldServer, StepReporting:
		return true
	}
	return false
}

// State is an install, as Updates/state.json records it. The app writes its
// first version, at StepWaitingForApp, before it starts hub-update and
// quits.
type State struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Installed is where the app is installed, ~/Applications/Job Search
	// Hub.app, and NewApp the checked download of To.
	Installed string `json:"installed"`
	NewApp    string `json:"new_app"`
	// ReopenApp says the app quit for this install, so it opens again
	// once the server is up. After Install when I quit, it stays closed.
	ReopenApp bool `json:"reopen_app"`
	// FromMigration and ToMigration are the newest migrations of the two
	// versions' servers; the dump taken before migrating from FromMigration
	// is the one a rollback restores.
	FromMigration int64     `json:"from_migration"`
	ToMigration   int64     `json:"to_migration"`
	StartedAt     time.Time `json:"started_at"`
	// JobPlist is the launchd job that runs hub-update, which goes once
	// the install has ended.
	JobPlist string `json:"job_plist,omitempty"`

	Step Step `json:"step"`
	// ServerLog is where the server's log stood when the new server started,
	// so the check reads only what it wrote.
	ServerLog int64 `json:"server_log"`
	// Migrating says the new server's log shows its migrations running, so
	// the steps window says "Updating the database…".
	Migrating bool `json:"migrating,omitempty"`
	// Runs counts the runs of hub-update on this install; one that keeps
	// crashing gives up.
	Runs int `json:"runs"`

	// Failure says why the install rolls back, in the owner's words.
	Failure string `json:"failure,omitempty"`
	// Dump is the pre-migration dump a rollback restores; empty when no
	// migration ran.
	Dump string `json:"dump,omitempty"`
	// Lost says what the rollback lost: "Nothing was lost." and so on.
	Lost string `json:"lost,omitempty"`
	// Error and Commands say where a rollback stopped, and how to finish it.
	Error    string   `json:"error,omitempty"`
	Commands []string `json:"commands,omitempty"`

	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// LoadState reads the state at path.
func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("read %s: %w", path, err)
	}
	return state, nil
}

// Save writes the state to path through a file renamed into place, so a
// reader never sees half of it.
func (state State) Save(path string) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	partial := path + ".partial"
	if err := os.WriteFile(partial, data, 0o600); err != nil {
		return err
	}
	return os.Rename(partial, path)
}

// validate refuses a state hub-update can't act on.
func (state State) validate() error {
	var missing []string
	for name, value := range map[string]string{"from": state.From, "to": state.To, "installed": state.Installed, "new_app": state.NewApp, "step": string(state.Step)} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		return fmt.Errorf("the install's state lacks %v", missing)
	}
	if state.StartedAt.IsZero() {
		return errors.New("the install's state lacks started_at")
	}
	return nil
}
