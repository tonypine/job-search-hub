package postgresprocess

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSegmentIDsAreReadFromIpcsAndProc(t *testing.T) {
	ipcs := `IPC status from <running system> as of Tue Oct  6 15:03:39 -03 2026
T     ID     KEY        MODE       OWNER    GROUP
Shared Memory:
m  65536 0x0052e2c1 --rw-------    owner    staff
m 131073 0x00000000 --rw-------    owner    staff
`
	if ids, err := parseIpcsIDs(ipcs); err != nil || !slices.Equal(ids, []int{65536, 131073}) {
		t.Errorf("ipcs: %v, %v", ids, err)
	}
	if ids, err := parseIpcsIDs("T     ID     KEY        MODE       OWNER    GROUP\nShared Memory:\n"); err != nil || len(ids) != 0 {
		t.Errorf("ipcs with no segments: %v, %v", ids, err)
	}
	if _, err := parseIpcsIDs("m  x 0x0 --rw------- owner staff\n"); err == nil {
		t.Error("an ipcs line without an ID was read")
	}

	proc := `       key      shmid perms                  size  cpid  lpid nattch   uid   gid  cuid  cgid      atime      dtime      ctime                   rss                  swap
   5432001          0   600                    56  1234  1234      6  1001  1001  1001  1001 1700000000          0 1700000000                  4096                     0
         0         32   600                  4096  1300     0      0  1001  1001  1001  1001          0          0 1700000000                     0                     0
`
	if ids, err := parseProcSysvipcIDs(proc); err != nil || !slices.Equal(ids, []int{0, 32}) {
		t.Errorf("/proc/sysvipc/shm: %v, %v", ids, err)
	}
}

func TestPostgresOutOfSharedMemorySegmentsIsExplained(t *testing.T) {
	output := `FATAL:  could not create shared memory segment: No space left on device
DETAIL:  Failed system call was shmget(key=5432001, size=56, 03600).`
	err := explainFullSharedMemory(errors.New("initdb: exit status 1"), output)
	if !strings.HasPrefix(err.Error(), sharedMemoryFull+": initdb: exit status 1") {
		t.Errorf("err = %v", err)
	}
	other := errors.New("initdb: exit status 1")
	if err := explainFullSharedMemory(other, "FATAL:  could not create lock file: No space left on device"); err != other {
		t.Errorf("another full disk is explained as %v", err)
	}
}

func TestALockFileNamesItsInterlockSegment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "postmaster.pid")
	for content, want := range map[string]int{
		"123\n/data\n1700000000\n5432\n/socket\n\n  5432001     65536\nready   \n": 65536,
		"123\n/data\n1700000000\n5432\n/socket\n\n  5432001         0\nready   \n": 0,
		"123\n/data\n1700000000\n5432\n/socket\n\n":                                -1,
		"123\n/data\n1700000000\n5432\n":                                           -1,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if file, _, err := readLockFile(path); err != nil || file.interlock != want {
			t.Errorf("%q: interlock %d, err %v; want %d", content, file.interlock, err, want)
		}
	}
}

// exitedInterlock has a child process create a 56-byte segment, as a
// postmaster creates its interlock segment, attaches the test to it and ends
// the child. It returns the child's PID, the segment's ID and a function that
// detaches the test. The segment is removed when the test ends, if it's still
// there.
func exitedInterlock(t *testing.T) (pid, id int, detach func()) {
	t.Helper()
	if _, err := RemoveOrphanedSegments(); err != nil {
		t.Fatalf("can't sweep the shared-memory segments here, so the test makes none: %v", err)
	}
	creator := exec.Command(os.Args[0])
	creator.Env = append(os.Environ(), "CREATE_SEGMENT_OF_BYTES=56")
	creator.Stderr = os.Stderr
	input, err := creator.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := creator.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := creator.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		input.Close()
		creator.Wait()
		t.Fatalf("the child didn't create a segment: %v", err)
	}
	if id, err = strconv.Atoi(strings.TrimSpace(line)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.SysvShmCtl(id, unix.IPC_RMID, nil) })
	memory, err := unix.SysvShmAttach(id, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	detached := false
	detach = func() {
		if !detached {
			detached = true
			unix.SysvShmDetach(memory)
		}
	}
	t.Cleanup(detach)
	input.Close()
	if err := creator.Wait(); err != nil {
		t.Fatal(err)
	}
	return creator.Process.Pid, id, detach
}

func TestAKilledPostmastersInterlockIsRemovedOnlyOnceNothingUsesIt(t *testing.T) {
	saved := interlockDetachTimeout
	interlockDetachTimeout = 200 * time.Millisecond
	t.Cleanup(func() { interlockDetachTimeout = saved })

	pid, id, detach := exitedInterlock(t)
	dir := t.TempDir()
	writeLockFile := func(pid int) {
		t.Helper()
		content := fmt.Sprintf("%d\n/data\n1700000000\n5432\n/s\n\n  0  %d\n", pid, id)
		if err := os.WriteFile(filepath.Join(dir, "postmaster.pid"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writeLockFile(pid)
	removeInterlock(dir, pid)
	if _, err := statSegment(id); err != nil {
		t.Fatalf("removed the interlock segment while a process has it attached: %v", err)
	}
	detach()
	writeLockFile(pid + 1)
	removeInterlock(dir, pid+1)
	if _, err := statSegment(id); err != nil {
		t.Fatalf("removed a segment another process created: %v", err)
	}
	writeLockFile(pid)
	removeInterlock(dir, pid)
	if _, err := statSegment(id); err == nil {
		t.Error("the interlock segment is still there once nothing has it attached")
	}
}
