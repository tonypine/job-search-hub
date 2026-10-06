package postgresprocess

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// replacedInfix names the cluster a restore replaced:
	// <major>.replaced-<date>, kept for the owner to delete.
	replacedInfix  = ".replaced-"
	replacedLayout = "2006-01-02-150405"
	// restoreLinesKept is how many of pg_restore's last lines a failed
	// restore's error carries.
	restoreLinesKept = 10
)

// Restore replaces the cluster with one restored from dump, a pg_dump in its
// custom format. It takes the folder's lock, so it fails with ErrLocked while
// a server runs the cluster. It builds the new cluster in <major>.partial,
// after removing one a killed restore left, restores the dump into its hub
// database, and runs check on it, which may migrate it. Only then does it
// swap it in, keeping the old cluster as <major>.replaced-<date>, whose path
// it returns; "" means there was no cluster to replace. A restore that fails
// leaves the current cluster as it was. Progress, pg_restore's included, goes
// to progress.
func Restore(ctx context.Context, settings Settings, dump string, check func(ctx context.Context, databaseURL string) error, progress io.Writer) (replaced string, err error) {
	if _, err := os.Stat(dump); err != nil {
		return "", fmt.Errorf("read the dump: %w", err)
	}
	dump, err = filepath.Abs(dump)
	if err != nil {
		return "", err
	}
	engine, dir, lock, err := lockDir(settings)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	major, err := getMajor(engine)
	if err != nil {
		return "", err
	}
	if err := refuseNewerCluster(dir, major); err != nil {
		return "", err
	}
	dataDir := filepath.Join(dir, major)
	partial := dataDir + ".partial"

	// A killed server may have left its Postgres running on the cluster, and
	// a killed restore its own on the partial one; either holds the socket.
	for _, running := range []string{dataDir, partial} {
		if err := stopOrphan(ctx, engine, running, socketLockPath(dir)); err != nil {
			return "", err
		}
	}
	fmt.Fprintf(progress, "Creating a new cluster in %s\n", partial)
	if err := createCluster(ctx, engine, partial); err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			if removeErr := os.RemoveAll(partial); removeErr != nil {
				slog.Warn("the unfinished cluster is left; the next restore removes it", "data", partial, "error", removeErr)
			}
		}
	}()
	if err := restoreInto(ctx, engine, dir, partial, dump, check, progress); err != nil {
		return "", err
	}
	return swapIn(dataDir, partial)
}

// restoreInto starts the cluster in dataDir, restores dump into its hub
// database, checks it, and stops it.
func restoreInto(ctx context.Context, engine, dir, dataDir, dump string, check func(ctx context.Context, databaseURL string) error, progress io.Writer) error {
	cluster, err := run(ctx, engine, dir, dataDir, nil)
	if err != nil {
		return err
	}
	defer cluster.stopPostgres()

	fmt.Fprintf(progress, "Restoring %s\n", dump)
	var output tailBuffer
	command := exec.CommandContext(ctx, filepath.Join(engine, "bin", "pg_restore"),
		"--no-owner",
		"--no-privileges",
		"--exit-on-error",
		"--verbose",
		"--dbname="+cluster.URL(),
		dump,
	)
	command.Stdout = io.MultiWriter(progress, &output)
	command.Stderr = command.Stdout
	if err := command.Run(); err != nil {
		return fmt.Errorf("pg_restore: %w%s", err, output.lastLines(restoreLinesKept))
	}
	if check != nil {
		fmt.Fprintln(progress, "Checking the restored database")
		if err := check(ctx, cluster.URL()); err != nil {
			return fmt.Errorf("check the restored database: %w", err)
		}
	}
	return nil
}

// swapIn moves the cluster in dataDir aside, if there is one, and partial in
// its place.
func swapIn(dataDir, partial string) (replaced string, err error) {
	if _, err := os.Lstat(dataDir); err == nil {
		replaced = dataDir + replacedInfix + time.Now().Format(replacedLayout)
		if err := os.Rename(dataDir, replaced); err != nil {
			return "", fmt.Errorf("move the current cluster aside: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(partial, dataDir); err != nil {
		if replaced != "" {
			if backErr := os.Rename(replaced, dataDir); backErr != nil {
				return "", fmt.Errorf("move the restored cluster into place: %w; and move %s back to %s: %w", err, replaced, dataDir, backErr)
			}
		}
		return "", fmt.Errorf("move the restored cluster into place: %w", err)
	}
	return replaced, syncDir(filepath.Dir(dataDir))
}

// tailBuffer keeps what is written to it, to quote its last lines.
type tailBuffer struct {
	bytes.Buffer
}

// lastLines is the last count lines, on lines of their own after a colon, or
// nothing.
func (buffer *tailBuffer) lastLines(count int) string {
	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return ":\n" + strings.Join(lines, "\n")
}
