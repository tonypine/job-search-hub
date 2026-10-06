// Package postgresprocess runs one Postgres cluster as a child of the server:
// it creates the cluster, starts it, waits for it, stops one an earlier server
// left behind, and stops it again. It moves an older major's cluster to the
// engine's major, and replaces the cluster with one restored from a dump. The
// cluster listens only on a Unix socket in its folder, and only the owner's
// user can reach it.
package postgresprocess

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// User owns the cluster and every object in it; DatabaseName is the hub's
	// database in it.
	User         = "hub"
	DatabaseName = "hub"

	lockFileName = "hub-server.lock"
	port         = "5432"
	socketName   = ".s.PGSQL." + port
	// maxSocketPath is the longest socket path macOS takes: sun_path holds
	// 104 bytes with the terminating NUL.
	maxSocketPath = 103

	startTimeout = 30 * time.Second
	logLinesKept = 20
)

var (
	// stopTimeout is how long a fast shutdown, or initdb's own cleanup, may
	// take before it is cut short.
	stopTimeout = 20 * time.Second
	// quitTimeout is how long postgres may take to quit after SIGQUIT. It
	// gives its children 5 seconds before it kills them.
	quitTimeout = 10 * time.Second
)

// ErrLocked means another server, or a database command, holds the folder's
// lock and owns the cluster.
var ErrLocked = errors.New("another hub-server or database command owns this database")

type Settings struct {
	// Engine is a Postgres installation's folder, holding bin/initdb,
	// bin/postgres and bin/pg_ctl. The folder it sits in is the engines
	// folder: an orphan is stopped only if it runs a postgres from there.
	Engine string
	// Dir holds the lock, the socket and the clusters, one per major in
	// <Dir>/<major>.
	Dir string
	// Engines holds the engines by major, postgres-<major>/. Start finds the
	// engine of an older major's cluster here, to move it to Engine's major.
	Engines string
	// Backups is the folder Start dumps an older major's cluster to before
	// moving it, and whose newest dump it names when it can't.
	Backups string
}

// Cluster is a running Postgres the caller owns until Stop.
type Cluster struct {
	dir     string
	dataDir string
	major   int
	lock    *os.File
	command *exec.Cmd
	log     *recentLines
	// upgradeErr says why the cluster is an older major's, still.
	upgradeErr *UpgradeError

	// exited closes when postgres ends, whoever ended it.
	exited   chan struct{}
	exitErr  error
	stopping atomic.Bool
	stopOnce sync.Once
}

// Start takes the folder's lock, refuses a cluster of a newer major than the
// engine's, and refuses to start when a killed restore left the old cluster
// moved aside and none in its place. When there is only an older major's
// cluster, it moves it to the engine's major first (see upgrade), and runs the
// older one with its own engine if that fails. It creates the cluster if there
// is none, stops a Postgres an earlier server left running on it, and starts
// postgres as a child. It returns once the hub database accepts connections.
func Start(ctx context.Context, settings Settings) (*Cluster, error) {
	settings, lock, err := lockDir(settings)
	if err != nil {
		return nil, err
	}
	cluster, err := start(ctx, settings, lock)
	if err != nil {
		lock.Close()
		return nil, err
	}
	return cluster, nil
}

// lockDir makes settings' engine and folder absolute, creates the folder, and
// takes its lock, which the caller closes.
func lockDir(settings Settings) (Settings, *os.File, error) {
	var err error
	if settings.Engine, err = filepath.Abs(settings.Engine); err != nil {
		return Settings{}, nil, err
	}
	if settings.Dir, err = filepath.Abs(settings.Dir); err != nil {
		return Settings{}, nil, err
	}
	dir := settings.Dir
	if socket := filepath.Join(dir, socketName); len(socket) > maxSocketPath {
		return Settings{}, nil, fmt.Errorf("the database socket's path, %s, is %d characters; macOS allows %d, so move the database folder somewhere shorter", socket, len(socket), maxSocketPath)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Settings{}, nil, fmt.Errorf("create the database folder: %w", err)
	}
	lock, err := takeLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return Settings{}, nil, err
	}
	return settings, lock, nil
}

func start(ctx context.Context, settings Settings, lock *os.File) (*Cluster, error) {
	engine, dir := settings.Engine, settings.Dir
	major, err := getMajor(engine)
	if err != nil {
		return nil, err
	}
	if err := refuseNewerCluster(dir, major); err != nil {
		return nil, err
	}
	dataDir := filepath.Join(dir, major)
	if err := refuseHalfRestored(dataDir); err != nil {
		return nil, err
	}
	oldMajor, err := findOlderCluster(dataDir)
	if err != nil {
		return nil, err
	}
	if oldMajor != "" {
		oldEngine, err := findEngine(settings.Engines, oldMajor)
		if err != nil {
			return nil, err
		}
		if oldEngine == "" {
			return nil, refuseWithoutOldEngine(dir, settings.Backups, oldMajor, major)
		}
		if err := upgrade(ctx, oldEngine, engine, dir, settings.Backups, oldMajor, major); err != nil {
			return startOlder(ctx, oldEngine, dir, oldMajor, major, lock, err)
		}
	}
	if err := createIfMissing(ctx, engine, dataDir); err != nil {
		return nil, err
	}
	return startCluster(ctx, engine, dir, major, lock)
}

// startCluster stops the Postgres an earlier server left running on the
// cluster of major in dir, if there is one, and runs the cluster with engine.
func startCluster(ctx context.Context, engine, dir, major string, lock *os.File) (*Cluster, error) {
	dataDir := filepath.Join(dir, major)
	if err := stopOrphan(ctx, engine, dataDir, socketLockPath(dir)); err != nil {
		return nil, err
	}
	cluster, err := run(ctx, engine, dir, dataDir, lock)
	if err != nil {
		return nil, err
	}
	cluster.major, _ = strconv.Atoi(major)
	return cluster, nil
}

func socketLockPath(dir string) string {
	return filepath.Join(dir, socketName+".lock")
}

// run starts postgres on dataDir, with its socket in dir, and waits until the
// hub database takes connections. The cluster's Stop closes lock.
func run(ctx context.Context, engine, dir, dataDir string, lock *os.File) (*Cluster, error) {
	if err := clearStaleSocketLock(engine, socketLockPath(dir)); err != nil {
		return nil, err
	}

	command := exec.Command(filepath.Join(engine, "bin", "postgres"),
		"-D", dataDir,
		"-c", "listen_addresses=",
		"-c", "port="+port,
		"-c", "unix_socket_directories="+dir,
		"-c", "unix_socket_permissions=0700",
		"-c", "max_connections=30",
	)
	// The write end goes to postgres and its backends; Wait doesn't wait for
	// it, so a crash is seen even while a dying backend still holds it.
	logReader, logWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	command.Stderr = logWriter
	if err := command.Start(); err != nil {
		logReader.Close()
		logWriter.Close()
		return nil, fmt.Errorf("start postgres: %w", err)
	}
	logWriter.Close()
	cluster := &Cluster{dir: dir, dataDir: dataDir, lock: lock, command: command, log: &recentLines{}, exited: make(chan struct{})}
	logDone := make(chan struct{})
	go func() {
		cluster.log.copyToSlog(logReader)
		logReader.Close()
		close(logDone)
	}()
	go func() {
		waitErr := command.Wait()
		if !cluster.stopping.Load() {
			// Give the reader a moment to take the last lines it wrote.
			select {
			case <-logDone:
			case <-time.After(2 * time.Second):
			}
			cluster.exitErr = cluster.describeExit(waitErr)
			slog.Error("postgres stopped by itself", "error", cluster.exitErr)
		}
		close(cluster.exited)
	}()
	slog.Info("postgres starting", "data", dataDir, "pid", command.Process.Pid)

	if err := cluster.waitUntilReady(ctx); err != nil {
		cluster.stopPostgres()
		return nil, err
	}
	slog.Info("postgres ready", "data", dataDir)
	return cluster, nil
}

// URL is the hub database's connection URL, for pgx and pg_dump.
func (cluster *Cluster) URL() string {
	return databaseURL(cluster.dir, DatabaseName)
}

// Major is the cluster's Postgres major.
func (cluster *Cluster) Major() int {
	return cluster.major
}

// UpgradeError says why Start ran an older major's cluster rather than move
// it to the engine's major; nil when it ran the engine's.
func (cluster *Cluster) UpgradeError() *UpgradeError {
	return cluster.upgradeErr
}

// DataDir is the cluster's data directory.
func (cluster *Cluster) DataDir() string {
	return cluster.dataDir
}

// PID is the postmaster's process ID.
func (cluster *Cluster) PID() int {
	return cluster.command.Process.Pid
}

// Exited closes when postgres has exited, after Stop or on its own.
func (cluster *Cluster) Exited() <-chan struct{} {
	return cluster.exited
}

// ExitError says why postgres exited on its own. It is nil while postgres
// runs and after Stop stopped it.
func (cluster *Cluster) ExitError() error {
	select {
	case <-cluster.exited:
		return cluster.exitErr
	default:
		return nil
	}
}

// Stop shuts postgres down, fast (SIGINT); after 20 seconds, immediately
// (SIGQUIT, recovered from the WAL at the next start); and after 10 more, as
// one that is suspended or stuck never takes either, with SIGKILL. Then it
// releases the folder's lock.
func (cluster *Cluster) Stop() {
	cluster.stopOnce.Do(func() {
		cluster.stopPostgres()
		cluster.lock.Close()
	})
}

func (cluster *Cluster) stopPostgres() {
	cluster.stopping.Store(true)
	select {
	case <-cluster.exited:
	default:
		_ = cluster.command.Process.Signal(syscall.SIGINT)
		select {
		case <-cluster.exited:
		case <-time.After(stopTimeout):
			slog.Warn("postgres didn't stop within the timeout; quitting it", "timeout", stopTimeout.String())
			_ = cluster.command.Process.Signal(syscall.SIGQUIT)
			select {
			case <-cluster.exited:
			case <-time.After(quitTimeout):
				slog.Warn("postgres didn't quit within the timeout; killing it", "timeout", quitTimeout.String())
				_ = cluster.command.Process.Kill()
				<-cluster.exited
			}
		}
		slog.Info("postgres stopped", "data", cluster.dataDir)
	}
}

// waitUntilReady waits for postgres to take a connection, then creates the
// hub's database once. Only the connection is retried: a first start's
// CREATE DATABASE copies template1, which on a slow disk can take longer than
// an attempt, and cancelling it would only start the copy over.
func (cluster *Cluster) waitUntilReady(ctx context.Context) error {
	connection, err := cluster.connect(ctx)
	if err != nil {
		return err
	}
	defer connection.Close(context.Background())
	if err := createHubDatabase(ctx, connection); err != nil {
		return fmt.Errorf("create the %s database: %w%s", DatabaseName, err, cluster.log.String())
	}
	return nil
}

// connect connects to the maintenance database as soon as postgres takes
// connections, giving each attempt 2 seconds.
func (cluster *Cluster) connect(ctx context.Context) (*pgx.Conn, error) {
	deadline := time.After(startTimeout)
	for {
		attempt, cancel := context.WithTimeout(ctx, 2*time.Second)
		connection, err := pgx.Connect(attempt, databaseURL(cluster.dir, "postgres"))
		cancel()
		if err == nil {
			return connection, nil
		}
		select {
		case <-cluster.exited:
			return nil, fmt.Errorf("postgres exited while starting: %w", cluster.exitErr)
		case <-deadline:
			return nil, fmt.Errorf("postgres didn't accept connections within %s: %v%s", startTimeout, err, cluster.log.String())
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// createHubDatabase creates the hub's database, which initdb doesn't, if it
// isn't there yet.
func createHubDatabase(ctx context.Context, connection *pgx.Conn) error {
	var exists bool
	if err := connection.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", DatabaseName).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err := connection.Exec(ctx, "CREATE DATABASE "+DatabaseName)
	return err
}

func (cluster *Cluster) describeExit(waitErr error) error {
	reason := "exited"
	if waitErr != nil {
		reason = waitErr.Error()
	}
	return fmt.Errorf("postgres %s%s", reason, cluster.log.String())
}

// databaseURL escapes a space as %20, not +, which libpq would keep as a plus.
func databaseURL(dir, database string) string {
	host := strings.ReplaceAll(url.QueryEscape(dir), "+", "%20")
	return "postgres:///" + database + "?host=" + host + "&user=" + User
}

// takeLock holds an exclusive flock on path until the returned file closes,
// or the process ends, however it ends.
func takeLock(path string) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the database lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		holder, _ := io.ReadAll(lock)
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: %s is held by %s", ErrLocked, path, describeHolder(string(holder)))
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	// Say who holds it, for the one that's refused.
	if executable, err := os.Executable(); err == nil {
		_ = lock.Truncate(0)
		_, _ = lock.WriteAt(fmt.Appendf(nil, "%d %s\n", os.Getpid(), filepath.Base(executable)), 0)
	}
	return lock, nil
}

func describeHolder(holder string) string {
	pid, command, found := strings.Cut(strings.TrimSpace(holder), " ")
	if !found {
		return "another process"
	}
	return fmt.Sprintf("%s (PID %s)", command, pid)
}

var versionPattern = regexp.MustCompile(`\(PostgreSQL\) (\d+)`)

// getMajor asks the engine's postgres for its major version, which names
// the cluster's folder.
func getMajor(engine string) (string, error) {
	output, err := exec.Command(filepath.Join(engine, "bin", "postgres"), "--version").Output()
	if err != nil {
		return "", fmt.Errorf("ask the Postgres engine in %s for its version: %w", engine, err)
	}
	match := versionPattern.FindSubmatch(output)
	if match == nil {
		return "", fmt.Errorf("unexpected postgres --version output: %q", output)
	}
	return string(match[1]), nil
}

// createIfMissing runs initdb into a sibling folder and renames it into place
// only once initdb succeeds, so an interrupted initdb never leaves a folder
// that looks like a finished cluster.
func createIfMissing(ctx context.Context, engine, dataDir string) error {
	if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	slog.Info("creating the database cluster", "data", dataDir)
	partial := dataDir + ".partial"
	if err := createCluster(ctx, engine, partial); err != nil {
		return err
	}
	if err := os.Rename(partial, dataDir); err != nil {
		return fmt.Errorf("move the new cluster into place: %w", err)
	}
	// Without this, a power cut could undo the rename after the hub has
	// written to the cluster, and the next start would remove it as unfinished.
	return syncDir(filepath.Dir(dataDir))
}

// createCluster runs initdb into dataDir, after removing what an earlier one,
// killed mid-way, left there.
func createCluster(ctx context.Context, engine, dataDir string) error {
	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("remove an unfinished cluster: %w", err)
	}
	command := exec.CommandContext(ctx, filepath.Join(engine, "bin", "initdb"),
		"-D", dataDir,
		"--username="+User,
		"--auth-local=trust",
		"--encoding=UTF8",
		"--locale-provider=builtin",
		"--builtin-locale=C.UTF-8",
	)
	// SIGTERM lets initdb remove what it wrote; SIGKILL follows if it lingers.
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = stopTimeout
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("initdb: %w: %s", err, output)
	}
	if err := excludeFromBackups(dataDir); err != nil {
		slog.Warn("the database stays in Time Machine's backups", "data", dataDir, "error", err)
	}
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	return nil
}

// recentLines logs postgres's output and keeps its last lines for errors.
type recentLines struct {
	mutex sync.Mutex
	lines []string
}

func (recent *recentLines) copyToSlog(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		slog.Info(line, "process", "postgres")
		recent.mutex.Lock()
		recent.lines = append(recent.lines, line)
		if len(recent.lines) > logLinesKept {
			recent.lines = recent.lines[len(recent.lines)-logLinesKept:]
		}
		recent.mutex.Unlock()
	}
}

// String is the last lines, on lines of their own after a colon, or nothing.
func (recent *recentLines) String() string {
	recent.mutex.Lock()
	defer recent.mutex.Unlock()
	if len(recent.lines) == 0 {
		return ""
	}
	return "; its last log lines:\n" + strings.Join(recent.lines, "\n")
}
