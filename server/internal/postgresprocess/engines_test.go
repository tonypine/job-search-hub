package postgresprocess_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

// addFakeEngine puts a folder in engines whose bin/postgres only answers
// --version with major, which is as far as a start that refuses gets.
func addFakeEngine(t *testing.T, engines, name, major string) string {
	t.Helper()
	engine := filepath.Join(engines, name)
	if err := os.MkdirAll(filepath.Join(engine, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho 'postgres (PostgreSQL) " + major + ".6'\n"
	if err := os.WriteFile(filepath.Join(engine, "bin", "postgres"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestTheNewestEngineIsTheHighestMajorNamedPostgres(t *testing.T) {
	engines := t.TempDir()
	addFakeEngine(t, engines, "postgres-9", "9")
	addFakeEngine(t, engines, "postgres-17", "17")
	want := addFakeEngine(t, engines, "postgres-18", "18")
	// Neither a folder being unpacked or replaced, nor one without a
	// postgres, is an engine.
	addFakeEngine(t, engines, "postgres-19.partial", "19")
	addFakeEngine(t, engines, "postgres-20.old", "20")
	if err := os.MkdirAll(filepath.Join(engines, "postgres-21", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	engine, err := postgresprocess.NewestEngine(engines)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ = filepath.EvalSymlinks(want); engine != want {
		t.Fatalf("engine = %s, want %s", engine, want)
	}
}

func TestTheNewestEngineIsResolvedThroughSymlinks(t *testing.T) {
	installed := addFakeEngine(t, t.TempDir(), "18", "18")
	engines := t.TempDir()
	if err := os.Symlink(installed, filepath.Join(engines, "postgres-18")); err != nil {
		t.Fatal(err)
	}

	engine, err := postgresprocess.NewestEngine(engines)
	if want, _ := filepath.EvalSymlinks(installed); err != nil || engine != want {
		t.Fatalf("engine = %s, %v; want %s", engine, err, want)
	}
}

func TestNoEngineIsAnErrorThatSaysHowToGetOne(t *testing.T) {
	for name, engines := range map[string]string{"empty": t.TempDir(), "missing": filepath.Join(t.TempDir(), "engines")} {
		_, err := postgresprocess.NewestEngine(engines)
		if err == nil || !strings.Contains(err.Error(), engines) || !strings.Contains(err.Error(), "HUB_DATABASE_URL") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestAClusterNewerThanTheEngineIsRefused(t *testing.T) {
	engine := addFakeEngine(t, t.TempDir(), "postgres-18", "18")
	dir := newDir(t)
	// An older cluster, and folders that aren't finished clusters, don't count.
	for _, name := range []string{"17", "19.partial", "19.replaced-2030-01-01"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "19"), 0o700); err != nil {
		t.Fatal(err)
	}

	for attempt := range 2 {
		_, err := postgresprocess.Start(context.Background(), postgresprocess.Settings{Engine: engine, Dir: dir})
		if err == nil || errors.Is(err, postgresprocess.ErrLocked) || !strings.Contains(err.Error(), "rolled back") || !strings.Contains(err.Error(), filepath.Join(dir, "19")) {
			t.Fatalf("attempt %d: err = %v", attempt+1, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "18")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a cluster was created for the older engine: %v", err)
	}
}
