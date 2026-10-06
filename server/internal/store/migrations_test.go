package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/migrations"
)

func TestAReleaseRecordsItselfAsTheFirstToKnowTheNewestMigration(t *testing.T) {
	ctx := context.Background()
	pool := testdatabase.New(t)

	for _, release := range []string{"", "0.1.300", "0.1.301"} {
		if err := store.MigrateAs(ctx, pool, release); err != nil {
			t.Fatalf("migrate as %q: %v", release, err)
		}
	}
	var release string
	if err := pool.QueryRow(ctx, `SELECT release FROM migration_releases WHERE migration = $1`, migrations.Newest()).Scan(&release); err != nil || release != "0.1.300" {
		t.Fatalf("release = %q, %v; want the first release that migrated it", release, err)
	}
}

func TestADatabaseAheadOfTheServersMigrationsIsRefused(t *testing.T) {
	ctx := context.Background()
	pool := testdatabase.New(t)
	newest := migrations.Newest()

	// A newer release's migrations, applied as goose records them.
	ahead := newest + 3
	if _, err := pool.Exec(ctx, `INSERT INTO goose_db_version (version_id, is_applied) VALUES ($1, true), ($2, true), ($3, true)`, newest+1, newest+2, ahead); err != nil {
		t.Fatal(err)
	}
	err := store.Migrate(ctx, pool)
	var refused *store.DatabaseAheadError
	if !errors.As(err, &refused) {
		t.Fatalf("migrate = %v, want the database refused", err)
	}
	if want := fmt.Sprintf("The database is at migration %d; this server knows up to %d. Install a newer release, or restore a backup from before it.", ahead, newest); err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO migration_releases (migration, release) VALUES ($1, '0.1.320')`, ahead); err != nil {
		t.Fatal(err)
	}
	err = store.Migrate(ctx, pool)
	if want := fmt.Sprintf("The database is at migration %d; this server knows up to %d. Install 0.1.320 or later, or restore a backup from before it.", ahead, newest); err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}

	// Back at this server's newest migration, it starts normally.
	if _, err := pool.Exec(ctx, `DELETE FROM goose_db_version WHERE version_id > $1`, newest); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate an up-to-date database: %v", err)
	}
}
