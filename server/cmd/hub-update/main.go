// Command hub-update installs a new version of the Mac app's bundle once the
// app has quit: it stops the server, swaps the bundle, starts the server and
// checks it, reopens the app and checks it, and puts the old version back if
// either fails. The app writes the install's Updates/state.json and starts
// it as a launchd job, which runs it again after a crash, and at the next
// login after a power loss, until the install ends.
//
//	hub-update install <state.json>
//	hub-update --version
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
	"github.com/tonypine/job-search-hub/server/internal/hubupdate"
	"github.com/tonypine/job-search-hub/server/internal/serverenv"
)

const usage = "usage: hub-update install <state.json>, or hub-update --version"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 1 && args[0] == "--version":
		fmt.Fprintln(stdout, "hub-update", buildinfo.Version())
		return 0
	case len(args) == 2 && args[0] == "install":
		return install(args[1], stdout, stderr)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

// install runs the install state names to its end. It exits 0 once the
// install has ended, however it ended, so launchd leaves the job be; and
// non-zero when this run stopped part way, so launchd runs it again.
func install(statePath string, stdout, stderr io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "hub-update:", err)
		return 1
	}
	settings, err := serverenv.Read(serverenv.Path(home))
	if err != nil {
		fmt.Fprintln(stderr, "hub-update:", err)
		return 1
	}
	statePath, err = filepath.Abs(statePath)
	if err != nil {
		fmt.Fprintln(stderr, "hub-update:", err)
		return 1
	}
	system := newLaunchdSystem(home, settings)
	machine := &hubupdate.Machine{
		StatePath: statePath, Updates: filepath.Dir(statePath), Backups: backupsDir(home, settings),
		HubURL: system.hubURL, Service: system.service, System: system, Client: &http.Client{Timeout: 5 * time.Second},
		Now: time.Now, Poll: hubupdate.DefaultPoll, AppQuitTimeout: hubupdate.DefaultAppQuitTimeout,
		ServerTimeout: hubupdate.DefaultServerTimeout, AppTimeout: hubupdate.DefaultAppTimeout,
		MigrationGrace: hubupdate.DefaultMigrationGrace, MaxRuns: hubupdate.DefaultMaxRuns,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	state, err := machine.Run(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "hub-update:", err)
		return 1
	}
	fmt.Fprintf(stdout, "The install of %s ended: %s\n", state.To, state.Step)
	return 0
}

// backupsDir is where the server keeps its dumps, as hub-server reads it.
func backupsDir(home string, settings map[string]string) string {
	if dir := settings["HUB_BACKUPS_DIR"]; dir != "" {
		return dir
	}
	return filepath.Join(home, "Library", "Application Support", "JobSearchHub", "backups")
}
