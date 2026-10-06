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

// errMalformedLockFile means a lock file was read whole, but Postgres
// couldn't have written it, as when a crash cut it short. It names no process
// to check.
var errMalformedLockFile = errors.New("malformed lock file")

// readLockFile reads path; found is false when there is no file. An error
// that isn't errMalformedLockFile means the file couldn't be read, and says
// nothing about what it holds.
func readLockFile(path string) (file lockFile, found bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return lockFile{}, false, nil
	}
	if err != nil {
		return lockFile{}, true, fmt.Errorf("read %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		return lockFile{}, true, fmt.Errorf("%w: %s has %d lines", errMalformedLockFile, path, len(lines))
	}
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || pid <= 0 {
		return lockFile{}, true, fmt.Errorf("%w: %s names no PID: %q", errMalformedLockFile, path, lines[0])
	}
	started, err := strconv.ParseInt(strings.TrimSpace(lines[2]), 10, 64)
	if err != nil {
		return lockFile{}, true, fmt.Errorf("%w: %s has no start time: %q", errMalformedLockFile, path, lines[2])
	}
	return lockFile{pid: pid, started: time.Unix(started, 0)}, true, nil
}

func isAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// lockHolder is what the process a lock file names turned out to be.
type lockHolder int

const (
	// holderGone: no process has the PID any more.
	holderGone lockHolder = iota
	// holderOther: a live process that isn't the postmaster that wrote the
	// file. After a reboot, the PID can belong to anything.
	holderOther
	// holderPostmaster: the postmaster that wrote the file, still running.
	holderPostmaster
)

// getHolder says which lockHolder file names. An error means the process is
// alive but couldn't be inspected, so it could be a postmaster: the caller
// must neither signal it nor delete the file.
func getHolder(engine string, file lockFile) (lockHolder, error) {
	if !isAlive(file.pid) {
		return holderGone, nil
	}
	postmaster, err := isPostmaster(engine, file)
	if err != nil {
		// It may have exited between the two checks.
		if !isAlive(file.pid) {
			return holderGone, nil
		}
		return 0, err
	}
	if postmaster {
		return holderPostmaster, nil
	}
	return holderOther, nil
}

// isPostmaster says whether the live process file names is the postmaster
// that wrote it: a postgres binary from the engines folder, started when the
// file says. An error means the process couldn't be inspected.
func isPostmaster(engine string, file lockFile) (bool, error) {
	executable, started, err := getProcessInfo(file.pid)
	if err != nil {
		return false, err
	}
	if filepath.Base(executable) != "postgres" {
		return false, nil
	}
	engines := resolve(filepath.Dir(engine))
	if !strings.HasPrefix(resolve(executable), engines+string(filepath.Separator)) {
		return false, nil
	}
	return started.Sub(file.started).Abs() <= startTimeSlack, nil
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
//
// Deleting the file under a running postmaster would let a second one start
// on the same data and corrupt it, so whenever the file or its process can't
// be read, Start fails instead.
func stopOrphan(ctx context.Context, engine, dataDir, socketLock string) error {
	path := filepath.Join(dataDir, "postmaster.pid")
	file, found, err := readLockFile(path)
	if !found {
		return nil
	}
	if errors.Is(err, errMalformedLockFile) {
		return removeMalformedPidFile(engine, path, socketLock, err)
	}
	if err != nil {
		return fmt.Errorf("can't tell whether a Postgres is using the cluster: %w", err)
	}
	holder, err := getHolder(engine, file)
	switch {
	case err != nil:
		return fmt.Errorf("can't tell whether process %d, which %s names, is this cluster's Postgres: %w; stop it if it is, or remove the file if it isn't", file.pid, path, err)
	case holder == holderGone:
		return nil
	case holder == holderOther:
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

// removeMalformedPidFile deletes a postmaster.pid that names no process,
// which Postgres won't start beside, only when the socket's lock file shows
// no Postgres serving the folder: there is none, or it names a process that
// is gone or isn't a postmaster.
func removeMalformedPidFile(engine, path, socketLock string, malformed error) error {
	file, found, err := readLockFile(socketLock)
	if err != nil {
		return fmt.Errorf("%w, and so can't tell whether a Postgres is using the cluster: %w", malformed, err)
	}
	if found {
		holder, err := getHolder(engine, file)
		if err != nil {
			return fmt.Errorf("%w, and can't tell whether process %d, which %s names, is a Postgres using the cluster: %w", malformed, file.pid, socketLock, err)
		}
		if holder == holderPostmaster {
			return fmt.Errorf("%w, and a Postgres (PID %d) is using the socket in %s", malformed, file.pid, filepath.Dir(socketLock))
		}
	}
	slog.Warn("removing a malformed postmaster.pid: no Postgres is using the socket", "error", malformed)
	return removeStale(path)
}

// clearStaleSocketLock deletes the socket's lock file when it names a live
// process that isn't a postmaster, as postmaster.pid can after a reboot, for
// the same reason. One naming a live postmaster means another cluster is
// using the folder's socket; one that can't be read, or names no process,
// leaves no way to tell, and Postgres refuses it as well.
func clearStaleSocketLock(engine, path string) error {
	file, found, err := readLockFile(path)
	if !found {
		return nil
	}
	if err != nil {
		return fmt.Errorf("can't tell whether another Postgres is using the socket in %s: %w; remove the file if none is", filepath.Dir(path), err)
	}
	holder, err := getHolder(engine, file)
	switch {
	case err != nil:
		return fmt.Errorf("can't tell whether process %d, which %s names, is a Postgres: %w; stop it if it is, or remove the file if it isn't", file.pid, path, err)
	case holder == holderPostmaster:
		return fmt.Errorf("another Postgres (PID %d) is using the socket in %s", file.pid, filepath.Dir(path))
	case holder == holderOther:
		slog.Warn("removing a stale socket lock file: its PID is another process", "pid", file.pid)
		return removeStale(path)
	}
	return nil
}

func removeStale(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the stale %s: %w", path, err)
	}
	return nil
}
