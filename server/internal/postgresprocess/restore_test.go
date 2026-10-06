package postgresprocess_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

// dumpCluster dumps the cluster's hub database with the engine's pg_dump,
// as the nightly dumps do.
func dumpCluster(t *testing.T, cluster *postgresprocess.Cluster) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hub.dump")
	output, err := exec.Command(filepath.Join(getEngine(t), "bin", "pg_dump"), "--format=custom", "--file="+path, "--dbname="+cluster.URL()).CombinedOutput()
	if err != nil {
		t.Fatalf("pg_dump: %v: %s", err, output)
	}
	return path
}

// clusterWithDump is a stopped cluster whose note was 'from the dump' when it
// was dumped, and is 'after the dump' now.
func clusterWithDump(t *testing.T) (dir, dataDir, dump string) {
	t.Helper()
	dir = newDir(t)
	cluster := start(t, dir)
	connection := connect(t, cluster)
	execute(t, connection, "CREATE TABLE kept (note text); INSERT INTO kept VALUES ('from the dump')")
	dump = dumpCluster(t, cluster)
	execute(t, connection, "UPDATE kept SET note = 'after the dump'")
	connection.Close(context.Background())
	cluster.Stop()
	return dir, cluster.DataDir(), dump
}

func restore(t *testing.T, dir, dump string, check func(context.Context, string) error, progress *bytes.Buffer) (string, error) {
	t.Helper()
	return postgresprocess.Restore(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir}, dump, check, progress)
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s is there: %v", path, err)
	}
}

// assertOnlyCluster checks that dir holds dataDir and no cluster beside it.
func assertOnlyCluster(t *testing.T, dir, dataDir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != filepath.Base(dataDir) {
			t.Errorf("%s is left beside the cluster", entry.Name())
		}
	}
}

func TestARestoreBringsTheDumpsRowsBackAndKeepsTheOldCluster(t *testing.T) {
	dir, dataDir, dump := clusterWithDump(t)
	var progress bytes.Buffer
	checked := ""
	replaced, err := restore(t, dir, dump, func(ctx context.Context, databaseURL string) error {
		checked = databaseURL
		return nil
	}, &progress)
	if err != nil {
		t.Fatalf("restore: %v\n%s", err, progress.String())
	}
	if !strings.HasPrefix(replaced, dataDir+".replaced-") {
		t.Fatalf("replaced = %q, want %s.replaced-<date>", replaced, dataDir)
	}
	if _, err := os.Stat(filepath.Join(replaced, "PG_VERSION")); err != nil {
		t.Fatalf("the old cluster isn't kept: %v", err)
	}
	if checked == "" {
		t.Error("the restored database wasn't checked")
	}
	if !strings.Contains(progress.String(), "pg_restore: ") {
		t.Errorf("pg_restore's progress isn't shown:\n%s", progress.String())
	}
	assertMissing(t, dataDir+".partial")

	cluster := start(t, dir)
	if note := queryString(t, connect(t, cluster), "SELECT note FROM kept"); note != "from the dump" {
		t.Fatalf("after the restore: %q", note)
	}
	if owner := queryString(t, connect(t, cluster), "SELECT tableowner::text FROM pg_tables WHERE tablename = 'kept'"); owner != postgresprocess.User {
		t.Errorf("the restored table is owned by %q", owner)
	}
}

func TestARestoreWhileTheLockIsHeldIsRefused(t *testing.T) {
	dir := newDir(t)
	cluster := start(t, dir)
	execute(t, connect(t, cluster), "CREATE TABLE kept (note text)")
	dump := dumpCluster(t, cluster)

	_, err := restore(t, dir, dump, nil, &bytes.Buffer{})
	if !errors.Is(err, postgresprocess.ErrLocked) {
		t.Fatalf("a restore while the server runs: %v", err)
	}
	if got := queryString(t, connect(t, cluster), "SELECT 'serving'"); got != "serving" {
		t.Fatal("the running cluster stopped serving")
	}
	assertOnlyCluster(t, dir, cluster.DataDir())
}

func TestARestoreOfABrokenDumpLeavesTheCurrentClusterUntouched(t *testing.T) {
	dir, dataDir, dump := clusterWithDump(t)
	broken := filepath.Join(t.TempDir(), "broken.dump")
	whole, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, whole[:len(whole)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := restore(t, dir, broken, nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "pg_restore") {
		t.Fatalf("a broken dump: %v", err)
	}
	assertOnlyCluster(t, dir, dataDir)

	// A dump that restores but fails the check is refused the same way.
	failed := errors.New("not the hub's")
	if _, err := restore(t, dir, dump, func(context.Context, string) error { return failed }, &bytes.Buffer{}); !errors.Is(err, failed) {
		t.Fatalf("a dump that fails its check: %v", err)
	}
	assertOnlyCluster(t, dir, dataDir)

	cluster := start(t, dir)
	if note := queryString(t, connect(t, cluster), "SELECT note FROM kept"); note != "after the dump" {
		t.Fatalf("the current cluster changed: %q", note)
	}
}

func TestARestoreAfterAKilledRestoresPartialClusterSucceeds(t *testing.T) {
	dir, dataDir, dump := clusterWithDump(t)
	// What a killed restore leaves: a cluster half restored, or half made.
	partial := dataDir + ".partial"
	if err := os.MkdirAll(filepath.Join(partial, "base"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partial, "PG_VERSION"), []byte(filepath.Base(dataDir)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var progress bytes.Buffer
	if _, err := restore(t, dir, dump, nil, &progress); err != nil {
		t.Fatalf("restore: %v\n%s", err, progress.String())
	}
	assertMissing(t, partial)
	cluster := start(t, dir)
	if note := queryString(t, connect(t, cluster), "SELECT note FROM kept"); note != "from the dump" {
		t.Fatalf("after the restore: %q", note)
	}
}

func TestARestoreWithNoClusterYetPutsOneInPlace(t *testing.T) {
	_, dataDir, dump := clusterWithDump(t)
	dir := newDir(t)
	var progress bytes.Buffer
	replaced, err := restore(t, dir, dump, nil, &progress)
	if err != nil {
		t.Fatalf("restore: %v\n%s", err, progress.String())
	}
	if replaced != "" {
		t.Fatalf("replaced %q, with no cluster to replace", replaced)
	}
	cluster := start(t, dir)
	if cluster.DataDir() != filepath.Join(dir, filepath.Base(dataDir)) {
		t.Fatalf("DataDir = %s", cluster.DataDir())
	}
	if note := queryString(t, connect(t, cluster), "SELECT note FROM kept"); note != "from the dump" {
		t.Fatalf("after the restore: %q", note)
	}
}

func TestAStartWithOnlyAReplacedClusterIsRefused(t *testing.T) {
	dir, dataDir := newStoppedCluster(t)
	// What a restore killed between its two renames leaves.
	replaced := dataDir + ".replaced-2026-10-06-120000"
	if err := os.Rename(dataDir, replaced); err != nil {
		t.Fatal(err)
	}

	err := startExpectingFailure(t, dir)
	if !strings.Contains(err.Error(), "rename "+replaced+" back to "+dataDir) {
		t.Fatalf("the refusal doesn't name the folder to rename back: %v", err)
	}
	assertMissing(t, dataDir)
	assertMissing(t, dataDir+".partial")
}

func own(t *testing.T, dir string) *postgresprocess.Owner {
	t.Helper()
	owner, err := postgresprocess.Own(postgresprocess.Settings{Engine: getEngine(t), Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Release)
	return owner
}

func TestInspectSeesTheCurrentClustersRowsAndHoldsTheLockUntilReleased(t *testing.T) {
	dir, _, _ := clusterWithDump(t)
	owner := own(t, dir)
	note := ""
	found, err := owner.Inspect(context.Background(), func(ctx context.Context, databaseURL string) error {
		note = queryString(t, connectTo(t, databaseURL), "SELECT note FROM kept")
		return nil
	})
	if err != nil || !found {
		t.Fatalf("Inspect: found %v, %v", found, err)
	}
	if note != "after the dump" {
		t.Fatalf("Inspect saw %q", note)
	}

	// The cluster is stopped again, and the lock still held.
	if _, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir}); !errors.Is(err, postgresprocess.ErrLocked) {
		t.Fatalf("a start while a command owns the folder: %v", err)
	}
	owner.Release()
	if note := queryString(t, connect(t, start(t, dir)), "SELECT note FROM kept"); note != "after the dump" {
		t.Fatalf("after Inspect: %q", note)
	}
}

func TestInspectFindsNoClusterInANewFolder(t *testing.T) {
	dir := newDir(t)
	found, err := own(t, dir).Inspect(context.Background(), func(context.Context, string) error {
		t.Fatal("inspected a cluster that isn't there")
		return nil
	})
	if err != nil || found {
		t.Fatalf("Inspect: found %v, %v", found, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Errorf("Inspect created %s", entry.Name())
		}
	}
}

func TestInspectRefusesAFolderAKilledRestoreLeftHalfSwapped(t *testing.T) {
	dir, dataDir := newStoppedCluster(t)
	replaced := dataDir + ".replaced-2026-10-06-120000"
	if err := os.Rename(dataDir, replaced); err != nil {
		t.Fatal(err)
	}
	_, err := own(t, dir).Inspect(context.Background(), func(context.Context, string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "rename "+replaced+" back to "+dataDir) {
		t.Fatalf("Inspect of a half-swapped folder: %v", err)
	}
}
