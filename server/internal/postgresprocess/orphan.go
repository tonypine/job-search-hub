package postgresprocess

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// startTimeSlack is how far a process's start time may be from the one its
// lock file records and still be the process that wrote it.
const startTimeSlack = 5 * time.Second

// lockFile is what Postgres writes to postmaster.pid, and to its socket's
// lock file: the postmaster's PID on the first line, and the time it started,
// in Unix seconds, on the third.
type lockFile struct {
	pid     int
	started time.Time
}

// readLockFile reads path; found is false when there is no file, and err
// non-nil when there is one Postgres couldn't have written whole.
func readLockFile(path string) (file lockFile, found bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return lockFile{}, false, nil
	}
	if err != nil {
		return lockFile{}, true, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		return lockFile{}, true, fmt.Errorf("%s has %d lines", path, len(lines))
	}
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || pid <= 0 {
		return lockFile{}, true, fmt.Errorf("%s names no PID: %q", path, lines[0])
	}
	started, err := strconv.ParseInt(strings.TrimSpace(lines[2]), 10, 64)
	if err != nil {
		return lockFile{}, true, fmt.Errorf("%s has no start time: %q", path, lines[2])
	}
	return lockFile{pid: pid, started: time.Unix(started, 0)}, true, nil
}

func isAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// isPostmaster says whether the live process file names is the postmaster
// that wrote it: a postgres binary from the engines folder, started when the
// file says. After a reboot, the PID can belong to anything.
func isPostmaster(engine string, file lockFile) bool {
	executable, started, err := getProcessInfo(file.pid)
	if err != nil {
		return false
	}
	if filepath.Base(executable) != "postgres" {
		return false
	}
	engines := resolve(filepath.Dir(engine))
	if !strings.HasPrefix(resolve(executable), engines+string(filepath.Separator)) {
		return false
	}
	return started.Sub(file.started).Abs() <= startTimeSlack
}

// resolve follows symlinks where it can, so paths compare as the kernel
// sees them.
func resolve(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// stopOrphan stops the Postgres an earlier server left running on dataDir,
// if postmaster.pid names one. Holding the folder's lock proves no server
// owns it. A pid file naming a live process that isn't that Postgres is
// stale, and is deleted, since Postgres won't start while it names a live
// process; the process is never signalled. One naming a dead process is
// left for Postgres, which replaces it.
func stopOrphan(ctx context.Context, engine, dataDir string) error {
	path := filepath.Join(dataDir, "postmaster.pid")
	file, found, err := readLockFile(path)
	if !found {
		return nil
	}
	if err != nil {
		slog.Warn("removing an unreadable postmaster.pid", "error", err)
		return removeStale(path)
	}
	if !isAlive(file.pid) {
		return nil
	}
	if !isPostmaster(engine, file) {
		slog.Warn("removing a stale postmaster.pid: its PID is another process", "pid", file.pid)
		return removeStale(path)
	}
	slog.Warn("stopping a Postgres an earlier server left running", "pid", file.pid, "data", dataDir)
	output, err := exec.CommandContext(ctx, filepath.Join(engine, "bin", "pg_ctl"), "stop", "-D", dataDir, "-m", "fast", "-w", "-t", "30").CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop the Postgres (PID %d) an earlier server left running: %w: %s", file.pid, err, output)
	}
	return nil
}

// clearStaleSocketLock deletes the socket's lock file when it names a live
// process that isn't a postmaster, as postmaster.pid can after a reboot, for
// the same reason. One naming a live postmaster means another cluster is
// using the folder's socket.
func clearStaleSocketLock(engine, path string) error {
	file, found, err := readLockFile(path)
	if !found {
		return nil
	}
	if err != nil {
		return removeStale(path)
	}
	if !isAlive(file.pid) {
		return nil
	}
	if isPostmaster(engine, file) {
		return fmt.Errorf("another Postgres (PID %d) is using the socket in %s", file.pid, filepath.Dir(path))
	}
	slog.Warn("removing a stale socket lock file: its PID is another process", "pid", file.pid)
	return removeStale(path)
}

func removeStale(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the stale %s: %w", path, err)
	}
	return nil
}
