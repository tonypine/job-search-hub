package postgresprocess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
