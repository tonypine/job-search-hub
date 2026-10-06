package postgresprocess

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
