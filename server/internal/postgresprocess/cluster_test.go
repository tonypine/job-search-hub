package postgresprocess_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
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

func connect(t *testing.T, cluster *postgresprocess.Cluster) *pgx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection, err := pgx.Connect(ctx, cluster.URL())
	if err != nil {
		t.Fatalf("connect to %s: %v", cluster.URL(), err)
	}
	t.Cleanup(func() { connection.Close(context.Background()) })
	return connection
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
	lockFile := fmt.Sprintf("%d\n%s\n%d\n5432\n%s\n\n  5432001         0\nready   \n", sleeper.Process.Pid, dataDir, time.Now().Unix(), dir)
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
	settings, err := os.OpenFile(filepath.Join(cluster.DataDir(), "postgresql.conf"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	settings.WriteString("this is not a setting\n")
	settings.Close()

	_, err = postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: getEngine(t), Dir: dir})
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
