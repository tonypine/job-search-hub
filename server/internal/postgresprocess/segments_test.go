package postgresprocess_test

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

// createSegmentUntilStdinCloses creates a segment of size bytes, prints its
// ID, and exits once its standard input closes, so the test decides when the
// segment's creator is gone.
func createSegmentUntilStdinCloses(size string) {
	bytes, err := strconv.Atoi(size)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	id, err := unix.SysvShmGet(unix.IPC_PRIVATE, bytes, unix.IPC_CREAT|0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shmget:", err)
		os.Exit(1)
	}
	fmt.Println(id)
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// canSweep is whether the segments can be listed here. A sandbox can allow
// shmget and refuse everything else, so a test makes no segment it may be
// unable to remove.
var canSweep = sync.OnceValue(func() error {
	_, err := postgresprocess.RemoveOrphanedSegments()
	return err
})

// newSegment has a child process create a segment of size bytes, and returns
// its ID and a function that ends the child, its creator. The segment is
// removed when the test ends, if it's still there.
func newSegment(t *testing.T, size int) (id int, endCreator func()) {
	t.Helper()
	if err := canSweep(); err != nil {
		t.Fatalf("can't sweep the shared-memory segments here, so the test makes none: %v", err)
	}
	creator := exec.Command(os.Args[0])
	creator.Env = append(os.Environ(), "CREATE_SEGMENT_OF_BYTES="+strconv.Itoa(size))
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
	endCreator = sync.OnceFunc(func() {
		input.Close()
		creator.Wait()
	})
	t.Cleanup(endCreator)
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		t.Fatalf("the child didn't create a segment: %v", err)
	}
	if id, err = strconv.Atoi(strings.TrimSpace(line)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.SysvShmCtl(id, unix.IPC_RMID, nil) })
	return id, endCreator
}

// orphanedSegment is a segment of size bytes whose creator has exited and
// that no process has attached, as a killed postmaster leaves its interlock
// segment.
func orphanedSegment(t *testing.T, size int) int {
	t.Helper()
	id, endCreator := newSegment(t, size)
	endCreator()
	return id
}

func segmentExists(id int) bool {
	var desc unix.SysvShmDesc
	_, err := unix.SysvShmCtl(id, unix.IPC_STAT, &desc)
	return err == nil
}

// interlockSegment is the ID of the running cluster's interlock segment, on
// postmaster.pid's seventh line after its key.
func interlockSegment(t *testing.T, cluster *postgresprocess.Cluster) int {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(cluster.DataDir(), "postmaster.pid"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) < 7 || len(strings.Fields(lines[6])) != 2 {
		t.Fatalf("postmaster.pid names no interlock segment: %q", content)
	}
	id, err := strconv.Atoi(strings.Fields(lines[6])[1])
	if err != nil {
		t.Fatal(err)
	}
	if !segmentExists(id) {
		t.Fatalf("the running cluster's interlock segment %d isn't there", id)
	}
	return id
}

func TestOnlyOrphanedInterlockSegmentsAreRemoved(t *testing.T) {
	orphan := orphanedSegment(t, 56)
	large := orphanedSegment(t, 4096)
	live, _ := newSegment(t, 56)
	attached, endCreator := newSegment(t, 56)
	memory, err := unix.SysvShmAttach(attached, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.SysvShmDetach(memory) })
	endCreator()

	if _, err := postgresprocess.RemoveOrphanedSegments(); err != nil {
		t.Fatal(err)
	}
	if segmentExists(orphan) {
		t.Error("the orphaned interlock segment is still there")
	}
	for name, id := range map[string]int{
		"larger than an interlock segment": large,
		"whose creator still runs":         live,
		"that a process has attached":      attached,
	} {
		if !segmentExists(id) {
			t.Errorf("removed a segment %s", name)
		}
	}
}

func TestStartRemovesAnOrphanedSegmentAndStartsTheCluster(t *testing.T) {
	orphan := orphanedSegment(t, 56)
	cluster := start(t, newDir(t))
	if segmentExists(orphan) {
		t.Error("the orphaned interlock segment is still there")
	}
	if got := queryString(t, connect(t, cluster), "SELECT 'serving'"); got != "serving" {
		t.Fatal("the cluster isn't serving")
	}
}
