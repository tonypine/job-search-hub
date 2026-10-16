package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/hubupdate"
)

const (
	serverLabel = "com.tonypine.jobsearchhub.server"
	// appProcess is the app's executable, which pgrep finds.
	appProcess = "JobSearchHub"
	// serverStopTimeout leaves room for the server's own stop: 30 seconds
	// for the work still running, 35 for llama-server, then Postgres.
	serverStopTimeout = 110 * time.Second
	appStopTimeout    = 10 * time.Second
)

// macCommands are the Mac's commands an install runs. The end-to-end test
// stands its own in, which run the bundles' servers without launchd.
var macCommands = struct {
	launchctl, pgrep, pkill, open string
}{"/bin/launchctl", "/usr/bin/pgrep", "/usr/bin/pkill", "/usr/bin/open"}

// launchdSystem is the Mac: launchd runs the server from the installed
// bundle, the app is found by its process, and the server's log is in
// ~/Library/Logs/JobSearchHub.
type launchdSystem struct {
	service    string
	domain     string
	plist      string
	serverLog  string
	hubURL     string
	ownerToken string
}

func newLaunchdSystem(home string, settings map[string]string) *launchdSystem {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	return &launchdSystem{
		service:    domain + "/" + serverLabel,
		domain:     domain,
		plist:      filepath.Join(home, "Library", "LaunchAgents", serverLabel+".plist"),
		serverLog:  filepath.Join(home, "Library", "Logs", "JobSearchHub", "server.log"),
		hubURL:     makeHubURL(settings["HUB_ADDR"]),
		ownerToken: settings["HUB_OWNER_TOKEN"],
	}
}

// makeHubURL is the server on this Mac, at HUB_ADDR's port.
func makeHubURL(address string) string {
	port := "8090"
	if index := strings.LastIndex(address, ":"); index >= 0 && index < len(address)-1 {
		port = address[index+1:]
	}
	return "http://127.0.0.1:" + port
}

func (system *launchdSystem) IsAppRunning(ctx context.Context) (bool, error) {
	err := exec.CommandContext(ctx, macCommands.pgrep, "-x", appProcess).Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	default:
		return false, err
	}
}

func (system *launchdSystem) QuitApp(ctx context.Context) error {
	_ = exec.CommandContext(ctx, macCommands.pkill, "-x", appProcess).Run()
	deadline := time.Now().Add(appStopTimeout)
	for time.Now().Before(deadline) {
		if running, err := system.IsAppRunning(ctx); err == nil && !running {
			return nil
		}
		time.Sleep(time.Second)
	}
	_ = exec.CommandContext(ctx, macCommands.pkill, "-9", "-x", appProcess).Run()
	if running, err := system.IsAppRunning(ctx); err != nil || running {
		return errors.New("the app didn't quit")
	}
	return nil
}

func (system *launchdSystem) OpenApp(ctx context.Context, app string) error {
	if output, err := exec.CommandContext(ctx, macCommands.open, app).CombinedOutput(); err != nil {
		return fmt.Errorf("open %s: %w: %s", app, err, bytes.TrimSpace(output))
	}
	return nil
}

// ShowSteps starts the bundle's steps window on its own: it outlives
// neither the install nor this run's need of it, and ends by itself.
func (system *launchdSystem) ShowSteps(_ context.Context, app, statePath string) error {
	steps := hubupdate.CommandPath(app, "hub-install-steps")
	if _, err := os.Stat(steps); err != nil {
		return nil
	}
	command := exec.Command(steps, statePath)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

// readServer is the agent's state in launchd: whether it's loaded, whether
// its server runs, and how it last exited.
func (system *launchdSystem) readServer(ctx context.Context) (loaded, running bool, lastExit string) {
	output, err := exec.CommandContext(ctx, macCommands.launchctl, "print", system.service).Output()
	if err != nil {
		return false, false, ""
	}
	// The service's own keys come first, one tab in; nested blocks repeat
	// some names deeper.
	for line := range strings.SplitSeq(string(output), "\n") {
		if !strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "\t\t") {
			continue
		}
		name, value, found := strings.Cut(strings.TrimSpace(line), " = ")
		if !found {
			continue
		}
		switch name {
		case "state":
			running = running || value == "running"
		case "last exit code":
			if lastExit == "" {
				lastExit = value
			}
		}
	}
	return true, running, lastExit
}

// StopServer sends SIGTERM, which the server answers by finishing its work
// and exiting 0, so launchd leaves it stopped. A server that exited with an
// error, as one crashing at start, launchd would start again: it's unloaded
// instead, and StartServer loads it.
func (system *launchdSystem) StopServer(ctx context.Context) error {
	loaded, running, _ := system.readServer(ctx)
	if !loaded {
		return nil
	}
	if running {
		if output, err := exec.CommandContext(ctx, macCommands.launchctl, "kill", "SIGTERM", system.service).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl kill: %w: %s", err, bytes.TrimSpace(output))
		}
	}
	deadline := time.Now().Add(serverStopTimeout)
	for {
		loaded, running, lastExit := system.readServer(ctx)
		if !loaded {
			return nil
		}
		if !running {
			if lastExit == "0" {
				return nil
			}
			return exec.CommandContext(ctx, macCommands.launchctl, "bootout", system.service).Run()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the server still runs after %v", serverStopTimeout)
		}
		if err := sleep(ctx, time.Second); err != nil {
			return err
		}
	}
}

func (system *launchdSystem) StartServer(ctx context.Context) error {
	var command *exec.Cmd
	if loaded, _, _ := system.readServer(ctx); loaded {
		command = exec.CommandContext(ctx, macCommands.launchctl, "kickstart", system.service)
	} else {
		command = exec.CommandContext(ctx, macCommands.launchctl, "bootstrap", system.domain, system.plist)
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(command.Args, " "), err, bytes.TrimSpace(output))
	}
	return nil
}

func (system *launchdSystem) ServerLogEnd() int64 {
	info, err := os.Stat(system.serverLog)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (system *launchdSystem) ReadServerLog(offset int64) ([]string, int64) {
	file, err := os.Open(system.serverLog)
	if err != nil {
		return nil, offset
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() < offset {
		// A new log since: read it from its start.
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset
	}
	data, err := io.ReadAll(io.LimitReader(file, 1<<20))
	if err != nil {
		return nil, offset
	}
	// Only whole lines; the rest is read next time.
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		return nil, offset
	}
	return strings.Split(string(data[:end]), "\n"), offset + int64(end) + 1
}

func (system *launchdSystem) CountWrites(ctx context.Context, hubServer string, migration int64) (hubupdate.WritesSinceMigration, error) {
	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(ctx, hubServer, "database", "count-writes", strconv.FormatInt(migration, 10))
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return hubupdate.WritesSinceMigration{}, fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	var counted hubupdate.WritesSinceMigration
	if err := json.Unmarshal(stdout.Bytes(), &counted); err != nil {
		return hubupdate.WritesSinceMigration{}, fmt.Errorf("read what count-writes printed: %w", err)
	}
	return counted, nil
}

func (system *launchdSystem) RestoreDatabase(ctx context.Context, hubServer, dump string) error {
	output, err := exec.CommandContext(ctx, hubServer, "database", "restore", dump).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(output))
	}
	return nil
}

// PostUpdate records the update through the server's POST /v1/updates, as
// the owner, so the app's feed and the phone hear about it.
func (system *launchdSystem) PostUpdate(ctx context.Context, title, body string) error {
	if system.ownerToken == "" {
		return errors.New("server.env has no HUB_OWNER_TOKEN")
	}
	payload, err := json.Marshal(map[string]string{"kind": "hub_version", "title": title, "body": body})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, system.hubURL+"/v1/updates", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+system.ownerToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("POST /v1/updates answered %d", response.StatusCode)
	}
	return nil
}

func sleep(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}
