package postgresprocess

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// maxInterlockSize is the largest System V segment taken for a Postgres
// interlock segment. Postgres keeps only its 56-byte header in one, to tell
// whether a cluster's processes still run; the shared memory itself is
// mmapped. It removes the segment on a clean or immediate shutdown only, so a
// postmaster that is killed leaves it behind, and macOS has room for just 32
// (kern.sysv.shmmni), which every cluster needs one of to start.
const maxInterlockSize = 64

// interlockDetachTimeout is how long a killed postmaster's children may take
// to notice and let go of its interlock segment before it's left in place.
var interlockDetachTimeout = 5 * time.Second

// segment is a System V shared-memory segment, as IPC_STAT reports it.
type segment struct {
	id       int
	owner    int
	size     int
	attached int
	creator  int
}

func statSegment(id int) (segment, error) {
	var desc unix.SysvShmDesc
	if _, err := unix.SysvShmCtl(id, unix.IPC_STAT, &desc); err != nil {
		return segment{}, err
	}
	return segment{
		id:       id,
		owner:    int(desc.Perm.Uid),
		size:     int(desc.Segsz),
		attached: int(desc.Nattch),
		creator:  int(desc.Cpid),
	}, nil
}

// isInterlockSized says whether the segment could be a Postgres interlock
// segment this user made.
func (segment segment) isInterlockSized() bool {
	return segment.owner == os.Getuid() && segment.size <= maxInterlockSize
}

// isOrphanedInterlock says whether nothing will use the segment again: no
// process has it attached, and the process that created it is gone.
func (segment segment) isOrphanedInterlock() bool {
	return segment.isInterlockSized() && segment.attached == 0 && segment.creator > 0 && !isAlive(segment.creator)
}

func removeSegment(id int) error {
	if _, err := unix.SysvShmCtl(id, unix.IPC_RMID, nil); err != nil {
		return fmt.Errorf("remove shared-memory segment %d: %w", id, err)
	}
	return nil
}

// RemoveOrphanedSegments removes the interlock segments killed Postgres
// clusters left behind: this user's segments of up to 64 bytes that no
// process has attached and whose creator no longer runs. It returns how many
// it removed. A segment it can't read is someone else's, or gone, and is
// skipped.
func RemoveOrphanedSegments() (removed int, err error) {
	ids, err := listSegments()
	if err != nil {
		return 0, fmt.Errorf("list the shared-memory segments: %w", err)
	}
	var failures []error
	for _, id := range ids {
		segment, err := statSegment(id)
		if err != nil || !segment.isOrphanedInterlock() {
			continue
		}
		if err := removeSegment(id); err != nil {
			failures = append(failures, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(failures...)
}

// sweepSegments runs RemoveOrphanedSegments before a cluster is created or
// started, and only logs what it couldn't do: the start can still find a
// free segment.
func sweepSegments() {
	removed, err := RemoveOrphanedSegments()
	if removed > 0 {
		slog.Info("removed shared-memory segments killed Postgres clusters left behind", "count", removed)
	}
	if err != nil {
		slog.Warn("can't remove the shared-memory segments killed Postgres clusters left behind", "error", err)
	}
}

// removeInterlock removes the interlock segment of the postmaster with pid,
// which has exited, once its children have let go of it. After a clean stop
// there is neither the segment nor postmaster.pid; after a kill there are
// both, and postmaster.pid names the segment. One still attached when the
// time is up is left: removing it would hide from the next start that the
// cluster's processes still run.
func removeInterlock(dataDir string, pid int) {
	file, found, err := readLockFile(filepath.Join(dataDir, "postmaster.pid"))
	if !found || err != nil || file.pid != pid || file.interlock < 0 {
		return
	}
	deadline := time.Now().Add(interlockDetachTimeout)
	for {
		segment, err := statSegment(file.interlock)
		if err != nil || segment.creator != pid || !segment.isInterlockSized() {
			return
		}
		if segment.attached == 0 {
			if err := removeSegment(segment.id); err != nil {
				slog.Warn("can't remove the shared-memory segment of a postgres that was killed", "pid", pid, "error", err)
				return
			}
			slog.Info("removed the shared-memory segment of a postgres that was killed", "pid", pid, "segment", segment.id)
			return
		}
		if time.Now().After(deadline) {
			slog.Warn("leaving the shared-memory segment of a postgres that was killed: its processes still use it", "pid", pid, "segment", segment.id, "attached", segment.attached)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// explainFullSharedMemory puts what's wrong in front of err when output, from
// initdb or postgres, shows it couldn't get a shared-memory segment because
// none is free.
func explainFullSharedMemory(err error, output string) error {
	if strings.Contains(output, "could not create shared memory segment") && strings.Contains(output, "No space left on device") {
		return fmt.Errorf("%s: %w", sharedMemoryFull, err)
	}
	return err
}

// parseIpcsIDs reads the IDs from macOS's ipcs -m, whose segment lines start
// with "m" and the ID.
func parseIpcsIDs(output string) ([]int, error) {
	var ids []int
	for line := range strings.Lines(output) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "m" {
			continue
		}
		id, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("unexpected ipcs line: %q", line)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// parseProcSysvipcIDs reads the IDs from Linux's /proc/sysvipc/shm: a header
// line, then one line per segment, its key and its ID first.
func parseProcSysvipcIDs(table string) ([]int, error) {
	var ids []int
	for line := range strings.Lines(table) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "key" {
			continue
		}
		id, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("unexpected /proc/sysvipc/shm line: %q", line)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
