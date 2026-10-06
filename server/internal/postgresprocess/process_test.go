package postgresprocess

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessInfoNamesTheExecutableAndWhenItStarted(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	sleep, _ = filepath.Abs(sleep)
	before := time.Now()
	sleeper := exec.Command(sleep, "60")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		sleeper.Process.Kill()
		sleeper.Wait()
	}()

	executable, started, err := getProcessInfo(sleeper.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if resolve(executable) != resolve(sleep) {
		t.Errorf("executable = %s, want %s", executable, sleep)
	}
	if started.Sub(before).Abs() > startTimeSlack {
		t.Errorf("started %s, %s from when it was started", started, started.Sub(before))
	}
	if isPostmaster(filepath.Dir(filepath.Dir(sleep)), lockFile{pid: sleeper.Process.Pid, started: started}) {
		t.Error("sleep passes for a postmaster")
	}
}

func TestALockFileIsReadOnlyWhenPostgresCouldHaveWrittenIt(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"empty":    "",
		"no pid":   "x\n/data\n1700000000\n",
		"no start": "123\n/data\n",
		"bad pid":  "-4\n/data\n1700000000\n",
	} {
		path := filepath.Join(dir, "postmaster.pid")
		os.WriteFile(path, []byte(content), 0o600)
		if _, found, err := readLockFile(path); !found || err == nil {
			t.Errorf("%s: found %t, err %v", name, found, err)
		}
	}
	path := filepath.Join(dir, "postmaster.pid")
	os.WriteFile(path, []byte("123\n/data\n1700000000\n5432\n"), 0o600)
	if file, found, err := readLockFile(path); err != nil || !found || file.pid != 123 || !file.started.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("a whole file: %+v, %t, %v", file, found, err)
	}
	if _, found, err := readLockFile(filepath.Join(dir, "missing")); found || err != nil {
		t.Errorf("no file: found %t, err %v", found, err)
	}
}
