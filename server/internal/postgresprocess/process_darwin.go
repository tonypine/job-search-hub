package postgresprocess

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"

	"golang.org/x/sys/unix"
)

// getProcessInfo returns the executable pid was started from and when it
// started, without cgo. kern.procargs2 begins with argc, 4 bytes, and then
// the path exec was given, which is what proc_pidpath reports for a process
// the server started with an absolute path.
func getProcessInfo(pid int) (executable string, started time.Time, err error) {
	arguments, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read process %d's arguments: %w", pid, err)
	}
	if len(arguments) < 5 {
		return "", time.Time{}, fmt.Errorf("process %d's arguments are %d bytes", pid, len(arguments))
	}
	path := arguments[4:]
	if end := bytes.IndexByte(path, 0); end >= 0 {
		path = path[:end]
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read process %d's start time: %w", pid, err)
	}
	if int(info.Proc.P_pid) != pid {
		return "", time.Time{}, fmt.Errorf("no process %d", pid)
	}
	start := info.Proc.P_starttime
	return string(path), time.Unix(start.Sec, int64(start.Usec)*1000), nil
}

// excludeFromBackups keeps Time Machine from copying path: a copy of a live
// cluster is the wrong thing to restore, and the nightly dumps are the
// backup. The exclusion is an attribute of the folder, so it moves with it.
func excludeFromBackups(path string) error {
	if output, err := exec.Command("/usr/bin/tmutil", "addexclusion", path).CombinedOutput(); err != nil {
		return fmt.Errorf("tmutil addexclusion: %w: %s", err, bytes.TrimSpace(output))
	}
	return nil
}
