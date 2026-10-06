package postgresprocess_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

// When OWN_POSTGRES_IN is set, this test binary acts as a server that owns
// the cluster in that folder: it starts it, prints the postmaster's PID, and
// waits to be killed, so a test can leave an orphan the way a crashed server
// does. It exits by itself once the test binary that started it is gone.
func TestMain(m *testing.M) {
	if dir := os.Getenv("OWN_POSTGRES_IN"); dir != "" {
		ownUntilKilled(dir)
		return
	}
	os.Exit(m.Run())
}

func ownUntilKilled(dir string) {
	cluster, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: os.Getenv("HUB_TEST_POSTGRES_ENGINE"), Dir: dir})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(cluster.PID())
	parent := os.Getppid()
	for range time.Tick(100 * time.Millisecond) {
		if os.Getppid() != parent {
			cluster.Stop()
			os.Exit(0)
		}
	}
}

func getEngine(t *testing.T) string {
	t.Helper()
	engine := os.Getenv("HUB_TEST_POSTGRES_ENGINE")
	if engine == "" {
		t.Fatal("HUB_TEST_POSTGRES_ENGINE is not set; point it at a Postgres 18 installation's folder, the one holding bin/postgres, e.g. /usr/lib/postgresql/18")
	}
	return engine
}

// newDir is a database folder short enough for the socket's path, which
// t.TempDir's, named after the test, can't promise.
func newDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "postgresprocess-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func start(t *testing.T, dir string) *postgresprocess.Cluster {
	t.Helper()
	cluster, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Stop)
	return cluster
}

// startExpectingFailure is Start where the test wants a refusal.
func startExpectingFailure(t *testing.T, dir string) error {
	t.Helper()
	cluster, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir})
	if err == nil {
		cluster.Stop()
		t.Fatal("the cluster started")
	}
	return err
}

// newStoppedCluster creates a cluster and stops it, for a test that leaves
// files in its way before the next start.
func newStoppedCluster(t *testing.T) (dir, dataDir string) {
	t.Helper()
	dir = newDir(t)
	cluster := start(t, dir)
	cluster.Stop()
	return dir, cluster.DataDir()
}

func connect(t *testing.T, cluster *postgresprocess.Cluster) *pgx.Conn {
	t.Helper()
	return connectTo(t, cluster.URL())
}

// connectTo connects to databaseURL, retrying while postgres starts.
func connectTo(t *testing.T, databaseURL string) *pgx.Conn {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		connection, err := pgx.Connect(ctx, databaseURL)
		cancel()
		if err == nil {
			t.Cleanup(func() { connection.Close(context.Background()) })
			return connection
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect to %s: %v", databaseURL, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// maintenanceURL is the URL of the cluster's postgres database, which is
// there before Start creates the hub's.
func maintenanceURL(dir string) string {
	return "postgres:///postgres?host=" + url.QueryEscape(dir) + "&user=hub"
}

func execute(t *testing.T, connection *pgx.Conn, statement string) {
	t.Helper()
	if _, err := connection.Exec(context.Background(), statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func appendSetting(t *testing.T, dataDir, setting string) {
	t.Helper()
	settings, err := os.OpenFile(filepath.Join(dataDir, "postgresql.conf"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer settings.Close()
	if _, err := settings.WriteString(setting + "\n"); err != nil {
		t.Fatal(err)
	}
}

// pidFileContent is a postmaster.pid as Postgres writes it.
func pidFileContent(pid int, dataDir, dir string) string {
	return fmt.Sprintf("%d\n%s\n%d\n5432\n%s\n\n  5432001         0\nready   \n", pid, dataDir, time.Now().Unix(), dir)
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s is gone: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func queryString(t *testing.T, connection *pgx.Conn, query string) string {
	t.Helper()
	var value string
	if err := connection.QueryRow(context.Background(), query).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

func isAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitUntilGone(t *testing.T, pid int) {
	t.Helper()
	for range 100 {
		if !isAlive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("process %d is still running", pid)
}

func TestAFreshClusterIsCreatedStartedAndStoppedLeavingNoProcess(t *testing.T) {
	dir := newDir(t)
	cluster := start(t, dir)
	if _, err := os.Stat(filepath.Join(cluster.DataDir(), "PG_VERSION")); err != nil {
		t.Fatalf("no cluster: %v", err)
	}
	if want := fmt.Sprintf("postgres:///hub?host=%s&user=hub", strings.ReplaceAll(dir, "/", "%2F")); cluster.URL() != want {
		t.Fatalf("URL = %s, want %s", cluster.URL(), want)
	}

	connection := connect(t, cluster)
	for query, want := range map[string]string{
		"SELECT current_user":          "hub",
		"SELECT current_database()":    "hub",
		"SHOW listen_addresses":        "",
		"SHOW max_connections":         "30",
		"SHOW unix_socket_permissions": "0700",
		"SHOW server_encoding":         "UTF8",
		"SELECT datlocprovider::text FROM pg_database WHERE datname = 'hub'": "b",
	} {
		if got := queryString(t, connection, query); got != want {
			t.Errorf("%s = %q, want %q", query, got, want)
		}
	}
	if _, err := connection.Exec(context.Background(), "CREATE TABLE kept (note text); INSERT INTO kept VALUES ('still here')"); err != nil {
		t.Fatal(err)
	}
	connection.Close(context.Background())

	pid := cluster.PID()
	cluster.Stop()
	select {
	case <-cluster.Exited():
	default:
		t.Fatal("Exited is open after Stop")
	}
	if err := cluster.ExitError(); err != nil {
		t.Fatalf("a stop by the owner is reported as %v", err)
	}
	waitUntilGone(t, pid)
	for _, path := range []string{filepath.Join(cluster.DataDir(), "postmaster.pid"), filepath.Join(dir, ".s.PGSQL.5432")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is left after a clean stop: %v", path, err)
		}
	}

	again := start(t, dir)
	if note := queryString(t, connect(t, again), "SELECT note FROM kept"); note != "still here" {
		t.Fatalf("after a restart: %q", note)
	}
}

func TestASecondOwnerIsRefusedByTheLock(t *testing.T) {
	dir := newDir(t)
	cluster := start(t, dir)
	_, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir})
	if !errors.Is(err, postgresprocess.ErrLocked) {
		t.Fatalf("a second owner: %v", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("PID %d", os.Getpid())) {
		t.Errorf("the error doesn't name the owner: %v", err)
	}
	if got := queryString(t, connect(t, cluster), "SELECT 'serving'"); got != "serving" {
		t.Fatal("the first owner's cluster stopped serving")
	}
}

func TestAnOrphanLeftByAKilledServerIsStoppedAndReplaced(t *testing.T) {
	dir := newDir(t)
	owner := exec.Command(os.Args[0])
	owner.Env = append(os.Environ(), "OWN_POSTGRES_IN="+dir, "HUB_TEST_POSTGRES_ENGINE="+getEngine(t))
	owner.Stderr = os.Stderr
	output, err := owner.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		owner.Process.Kill()
		owner.Wait()
		t.Fatalf("the owner didn't start its cluster: %v", err)
	}
	orphan, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if isAlive(orphan) {
			syscall.Kill(orphan, syscall.SIGQUIT)
		}
	})
	owner.Process.Kill()
	owner.Wait()
	if !isAlive(orphan) {
		t.Fatal("postgres died with its owner; there is no orphan to test")
	}

	cluster := start(t, dir)
	waitUntilGone(t, orphan)
	if cluster.PID() == orphan {
		t.Fatal("the orphan was kept")
	}
	if got := queryString(t, connect(t, cluster), "SELECT pg_postmaster_start_time() > now() - interval '1 minute'"); got != "true" {
		t.Fatal("not served by the new postgres")
	}
}

func TestAPidFileNamingAnotherLiveProcessIsLeftAloneAndTheClusterStarts(t *testing.T) {
	dir := newDir(t)
	cluster := start(t, dir)
	dataDir := cluster.DataDir()
	cluster.Stop()

	// After a reboot, the PID a crashed postmaster left can be anything's.
	sleeper := exec.Command("sleep", "600")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		sleeper.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		sleeper.Process.Kill()
		<-exited
	})
	lockFile := pidFileContent(sleeper.Process.Pid, dataDir, dir)
	for _, path := range []string{filepath.Join(dataDir, "postmaster.pid"), filepath.Join(dir, ".s.PGSQL.5432.lock")} {
		if err := os.WriteFile(path, []byte(lockFile), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	again := start(t, dir)
	if got := queryString(t, connect(t, again), "SELECT 'serving'"); got != "serving" {
		t.Fatal("the cluster isn't serving")
	}
	select {
	case <-exited:
		t.Fatal("the process the stale pid file named was signalled")
	default:
	}
}

func TestAChildThatCrashesIsReportedToTheCaller(t *testing.T) {
	cluster := start(t, newDir(t))
	if err := syscall.Kill(cluster.PID(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cluster.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("Exited didn't close after postgres crashed")
	}
	if err := cluster.ExitError(); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("the crash is reported as %v", err)
	}
}

func TestAChildThatFailsToStartIsReportedWithItsLastLogLines(t *testing.T) {
	dir := newDir(t)
	cluster := start(t, dir)
	cluster.Stop()
	appendSetting(t, cluster.DataDir(), "this is not a setting")

	_, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir})
	if err == nil || !strings.Contains(err.Error(), "postgresql.conf") {
		t.Fatalf("a postgres that can't read its settings: %v", err)
	}
	// The failed start released the lock.
	if lock, err := os.OpenFile(filepath.Join(dir, "hub-server.lock"), os.O_RDWR, 0); err != nil {
		t.Fatal(err)
	} else {
		defer lock.Close()
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatalf("the lock is still held: %v", err)
		}
	}
}

func TestASocketPathTooLongIsRefusedBeforePostgresStarts(t *testing.T) {
	dir := filepath.Join(newDir(t), strings.Repeat("d", 100))
	_, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir})
	if err == nil || !strings.Contains(err.Error(), "103") {
		t.Fatalf("a socket path over 103 characters: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the folder was created: %v", err)
	}
}

func TestAnInitdbLeftUnfinishedIsReplacedByAWorkingCluster(t *testing.T) {
	dir := newDir(t)
	output, err := exec.Command(filepath.Join(getEngine(t), "bin", "postgres"), "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	major := strings.SplitN(strings.Fields(string(output))[2], ".", 2)[0]
	// What a killed initdb leaves: PG_VERSION and no template1.
	unfinished := filepath.Join(dir, major+".partial")
	if err := os.MkdirAll(filepath.Join(unfinished, "base"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unfinished, "PG_VERSION"), []byte(major+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cluster := start(t, dir)
	if cluster.DataDir() != filepath.Join(dir, major) {
		t.Fatalf("DataDir = %s, want %s", cluster.DataDir(), filepath.Join(dir, major))
	}
	if got := queryString(t, connect(t, cluster), "SELECT current_database()"); got != "hub" {
		t.Fatalf("current_database() = %q", got)
	}
	if _, err := os.Stat(unfinished); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the unfinished folder is left: %v", err)
	}
}

func TestAnUnreadablePidFileStopsTheStartAndIsKept(t *testing.T) {
	dir, dataDir := newStoppedCluster(t)
	path := filepath.Join(dataDir, "postmaster.pid")
	content := pidFileContent(os.Getpid(), dataDir, dir)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}

	if err := startExpectingFailure(t, dir); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("a postmaster.pid that can't be read: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("the file is gone: %v", err)
	}
	assertContent(t, path, content)
}

func TestAPidFileNamingAProcessThatCantBeInspectedIsRefusedAndKept(t *testing.T) {
	dir, dataDir := newStoppedCluster(t)
	// PID 1 is root's, on Linux and on the Mac; the test's user can see that
	// it is alive, but not what it runs or when it started.
	path := filepath.Join(dataDir, "postmaster.pid")
	content := pidFileContent(1, dataDir, dir)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := startExpectingFailure(t, dir); !strings.Contains(err.Error(), "process 1,") {
		t.Fatalf("a postmaster.pid naming a process that can't be inspected: %v", err)
	}
	assertContent(t, path, content)
}

func TestASocketLockNamingAnotherLivePostgresIsRefusedAndKept(t *testing.T) {
	other := start(t, newDir(t))
	theirs, err := os.ReadFile(filepath.Join(other.DataDir(), "postmaster.pid"))
	if err != nil {
		t.Fatal(err)
	}
	dir := newDir(t)
	socketLock := filepath.Join(dir, ".s.PGSQL.5432.lock")
	if err := os.WriteFile(socketLock, theirs, 0o600); err != nil {
		t.Fatal(err)
	}

	err = startExpectingFailure(t, dir)
	if want := fmt.Sprintf("another Postgres (PID %d)", other.PID()); !strings.Contains(err.Error(), want) {
		t.Fatalf("a socket held by a live Postgres: %v, want %q", err, want)
	}
	assertContent(t, socketLock, string(theirs))
	if got := queryString(t, connect(t, other), "SELECT 'serving'"); got != "serving" {
		t.Fatal("the other Postgres stopped serving")
	}
}

func TestAMalformedPidFileIsKeptWhileAPostgresHoldsTheSocket(t *testing.T) {
	other := start(t, newDir(t))
	theirs, err := os.ReadFile(filepath.Join(other.DataDir(), "postmaster.pid"))
	if err != nil {
		t.Fatal(err)
	}
	dir, dataDir := newStoppedCluster(t)
	// What a crash while Postgres wrote it can leave.
	pidFile := filepath.Join(dataDir, "postmaster.pid")
	if err := os.WriteFile(pidFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	socketLock := filepath.Join(dir, ".s.PGSQL.5432.lock")
	if err := os.WriteFile(socketLock, theirs, 0o600); err != nil {
		t.Fatal(err)
	}

	err = startExpectingFailure(t, dir)
	if want := fmt.Sprintf("a Postgres (PID %d) is using the socket", other.PID()); !strings.Contains(err.Error(), want) {
		t.Fatalf("a malformed postmaster.pid beside a live Postgres: %v, want %q", err, want)
	}
	assertContent(t, pidFile, "")

	// With no Postgres on the socket, nothing can be using the cluster.
	if err := os.Remove(socketLock); err != nil {
		t.Fatal(err)
	}
	if got := queryString(t, connect(t, start(t, dir)), "SELECT 'serving'"); got != "serving" {
		t.Fatal("the cluster isn't serving")
	}
}

func TestACreateDatabaseSlowerThanAConnectionAttemptDoesntFailTheStart(t *testing.T) {
	dir, dataDir := newStoppedCluster(t)
	appendSetting(t, dataDir, "max_prepared_transactions = 1")
	cluster := start(t, dir)
	maintenance := connectTo(t, maintenanceURL(dir))
	execute(t, maintenance, "DROP DATABASE hub WITH (FORCE)")
	// A prepared transaction keeps its locks across a restart. This one
	// holds template1, which CREATE DATABASE copies, until the test lets go.
	execute(t, maintenance, "BEGIN")
	execute(t, maintenance, "COMMENT ON DATABASE template1 IS 'held'")
	execute(t, maintenance, "PREPARE TRANSACTION 'hold_template1'")
	maintenance.Close(context.Background())
	cluster.Stop()

	engine := getEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	var again *postgresprocess.Cluster
	var startErr error
	done := make(chan struct{})
	go func() {
		again, startErr = postgresprocess.Start(ctx, postgresprocess.Settings{Engine: engine, Dir: dir})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		if again != nil {
			again.Stop()
		}
	})

	watcher := connectTo(t, maintenanceURL(dir))
	var creator int
	for deadline := time.Now().Add(30 * time.Second); ; {
		err := watcher.QueryRow(context.Background(), "SELECT pid FROM pg_stat_activity WHERE query LIKE 'CREATE DATABASE%' AND wait_event_type = 'Lock'").Scan(&creator)
		if err == nil {
			break
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
		select {
		case <-done:
			t.Fatalf("Start returned before creating the database: %v", startErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("Start never waited on CREATE DATABASE")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Longer than a connection attempt's 2 seconds.
	time.Sleep(3 * time.Second)
	var stillWaiting bool
	if err := watcher.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = $1 AND wait_event_type = 'Lock')", creator).Scan(&stillWaiting); err != nil {
		t.Fatal(err)
	}
	if !stillWaiting {
		t.Fatal("the slow CREATE DATABASE was cancelled")
	}
	execute(t, watcher, "ROLLBACK PREPARED 'hold_template1'")

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Start didn't return once CREATE DATABASE could go ahead")
	}
	if startErr != nil {
		t.Fatal(startErr)
	}
	if got := queryString(t, connect(t, again), "SELECT current_database()"); got != "hub" {
		t.Fatalf("current_database() = %q", got)
	}
}

func TestAPidFileAndSocketLockThatNameNoProcessStopTheStartAndAreKept(t *testing.T) {
	dir, dataDir := newStoppedCluster(t)
	// What a crash while Postgres wrote both can leave.
	pidFile := filepath.Join(dataDir, "postmaster.pid")
	socketLock := filepath.Join(dir, ".s.PGSQL.5432.lock")
	for _, path := range []string{pidFile, socketLock} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := startExpectingFailure(t, dir); !strings.Contains(err.Error(), "can't tell whether a Postgres is using the cluster") {
		t.Fatalf("a malformed postmaster.pid beside a malformed socket lock: %v", err)
	}
	assertContent(t, pidFile, "")
	assertContent(t, socketLock, "")
}

func TestASocketLockThatCantBeReadStopsTheStartAndIsKept(t *testing.T) {
	for name, test := range map[string]struct {
		content string
		mode    os.FileMode
	}{
		"malformed":  {content: "not a pid\n", mode: 0o600},
		"unreadable": {content: pidFileContent(os.Getpid(), "/data", "/socket"), mode: 0},
	} {
		t.Run(name, func(t *testing.T) {
			dir, _ := newStoppedCluster(t)
			socketLock := filepath.Join(dir, ".s.PGSQL.5432.lock")
			if err := os.WriteFile(socketLock, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(socketLock, test.mode); err != nil {
				t.Fatal(err)
			}

			if err := startExpectingFailure(t, dir); !strings.Contains(err.Error(), "can't tell whether another Postgres is using the socket") {
				t.Fatalf("a %s socket lock: %v", name, err)
			}
			if err := os.Chmod(socketLock, 0o600); err != nil {
				t.Fatalf("the file is gone: %v", err)
			}
			assertContent(t, socketLock, test.content)
		})
	}
}

func TestStopKillsAPostgresThatIgnoresItsSignals(t *testing.T) {
	postgresprocess.ShortenStopTimeouts(t, time.Second, time.Second)
	cluster := start(t, newDir(t))
	pid := cluster.PID()
	// A suspended postmaster keeps SIGINT and SIGQUIT pending.
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })

	stopped := make(chan struct{})
	go func() {
		cluster.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(30 * time.Second):
		t.Fatal("Stop didn't return")
	}
	waitUntilGone(t, pid)
	if err := cluster.ExitError(); err != nil {
		t.Fatalf("a stop by the owner is reported as %v", err)
	}
}
