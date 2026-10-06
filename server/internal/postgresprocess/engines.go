package postgresprocess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// enginePattern names an engine in the engines folder by its major, as
	// the install script and app updates put it there. A folder being
	// unpacked or replaced is named otherwise, and isn't one.
	enginePattern = regexp.MustCompile(`^postgres-(\d+)$`)
	// clusterPattern names a finished cluster by its major. A cluster being
	// built (<major>.partial) or kept after a restore (<major>.replaced-…)
	// isn't one.
	clusterPattern = regexp.MustCompile(`^\d+$`)
)

// NewestEngine is the engine in engines with the highest major: a folder
// named postgres-<major> holding bin/postgres. Its symlinks are resolved, so
// a running postgres's executable compares with it as the kernel reports it.
func NewestEngine(engines string) (string, error) {
	entries, err := os.ReadDir(engines)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read the Postgres engines in %s: %w", engines, err)
	}
	newest, newestMajor := "", -1
	for _, entry := range entries {
		match := enginePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		major, err := strconv.Atoi(match[1])
		if err != nil || major <= newestMajor {
			continue
		}
		engine := filepath.Join(engines, entry.Name())
		if _, err := os.Stat(filepath.Join(engine, "bin", "postgres")); err != nil {
			continue
		}
		newest, newestMajor = engine, major
	}
	if newest == "" {
		return "", fmt.Errorf("no Postgres engine in %s: install one with server/scripts/install-native-server.sh, set HUB_POSTGRES_ENGINES to the folder holding postgres-<major>/, or set HUB_DATABASE_URL to use another Postgres", engines)
	}
	resolved, err := filepath.EvalSymlinks(newest)
	if err != nil {
		return "", fmt.Errorf("resolve the Postgres engine %s: %w", newest, err)
	}
	return resolved, nil
}

// refuseNewerCluster fails when dir holds a cluster of a newer major than the
// engine's: the app was rolled back past an upgrade, and the older cluster
// beside it, or a fresh one, would be missing everything written since.
func refuseNewerCluster(dir, major string) error {
	engineMajor, err := strconv.Atoi(major)
	if err != nil {
		return fmt.Errorf("the engine's major %q isn't a number", major)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read the database folder: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !clusterPattern.MatchString(entry.Name()) {
			continue
		}
		if clusterMajor, err := strconv.Atoi(entry.Name()); err == nil && clusterMajor > engineMajor {
			return fmt.Errorf("the database in %s is Postgres %d, newer than this hub's engine, Postgres %d: the hub was rolled back past a database upgrade; install the newer hub again", filepath.Join(dir, entry.Name()), clusterMajor, engineMajor)
		}
	}
	return nil
}

// refuseHalfRestored fails when there is no cluster in dataDir but a
// <major>.replaced-… folder beside it: a restore was stopped between moving
// the old cluster aside and moving the new one in, and a fresh cluster would
// hide the data in both.
func refuseHalfRestored(dataDir string) error {
	if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(filepath.Dir(dataDir))
	if err != nil {
		return fmt.Errorf("read the database folder: %w", err)
	}
	// The dates in the names sort in time, and ReadDir sorts by name.
	replaced := ""
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), filepath.Base(dataDir)+replacedInfix) {
			replaced = filepath.Join(filepath.Dir(dataDir), entry.Name())
		}
	}
	if replaced == "" {
		return nil
	}
	return fmt.Errorf("there is no database cluster in %s, but there is %s: a restore was stopped between moving that old cluster aside and moving the new one in; rename %s back to %s and start the hub again, or run hub-server database restore again", dataDir, replaced, replaced, dataDir)
}
