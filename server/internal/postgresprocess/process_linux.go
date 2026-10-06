package postgresprocess

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// clockTicks is USER_HZ, the unit of /proc/<pid>/stat's start time. It is
// 100 on every Linux the server runs on, and can't be read without cgo.
const clockTicks = 100

// getProcessInfo returns the executable pid runs and when it started.
func getProcessInfo(pid int) (executable string, started time.Time, err error) {
	executable, err = os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", time.Time{}, err
	}
	executable = strings.TrimSuffix(executable, " (deleted)")

	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", time.Time{}, err
	}
	// The command name, in parentheses, can hold spaces; the fields after
	// it start with the third, so the start time, the 22nd, is the 20th.
	closing := strings.LastIndexByte(string(stat), ')')
	if closing < 0 {
		return "", time.Time{}, fmt.Errorf("unexpected /proc/%d/stat: %q", pid, stat)
	}
	fields := strings.Fields(string(stat[closing+1:]))
	if len(fields) < 20 {
		return "", time.Time{}, fmt.Errorf("unexpected /proc/%d/stat: %q", pid, stat)
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("unexpected start time in /proc/%d/stat: %w", pid, err)
	}
	booted, err := getBootTime()
	if err != nil {
		return "", time.Time{}, err
	}
	return executable, booted.Add(time.Duration(ticks) * time.Second / clockTicks), nil
}

func getBootTime() (time.Time, error) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, err
	}
	for line := range strings.Lines(string(stat)) {
		if value, found := strings.CutPrefix(line, "btime "); found {
			seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("unexpected btime in /proc/stat: %w", err)
			}
			return time.Unix(seconds, 0), nil
		}
	}
	return time.Time{}, fmt.Errorf("no btime in /proc/stat")
}

// excludeFromBackups does nothing: Linux has no Time Machine.
func excludeFromBackups(string) error {
	return nil
}
