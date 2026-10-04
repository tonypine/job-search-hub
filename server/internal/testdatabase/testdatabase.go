// Package testdatabase gives each test its own freshly migrated Postgres
// database, dropped when the test ends.
package testdatabase

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// agentPrompts are made-up prompts of every kind: the real ones live only in
// the owner's database.
//
//go:embed agentprompts
var agentPrompts embed.FS

// New creates the database through HUB_TEST_DATABASE_URL, which points at the
// maintenance database of the test Postgres in compose.test.yaml.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	adminURL := os.Getenv("HUB_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("HUB_TEST_DATABASE_URL is not set; start the test Postgres with docker compose -f compose.test.yaml up -d --wait and set it to postgres://hub:hub-test@localhost:5435/postgres")
	}
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to the test Postgres: %v", err)
	}

	name := "hub_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
		admin.Close(ctx)
	})

	settings, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse HUB_TEST_DATABASE_URL: %v", err)
	}
	settings.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, settings)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(pool.Close)

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate %s: %v", name, err)
	}
	seed, err := fs.Sub(agentPrompts, "agentprompts")
	if err != nil {
		t.Fatalf("read the test prompts: %v", err)
	}
	if _, err := store.New(pool).SeedAgentPrompts(ctx, seed); err != nil {
		t.Fatalf("seed the test prompts in %s: %v", name, err)
	}
	return pool
}
