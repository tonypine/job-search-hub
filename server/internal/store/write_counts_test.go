package store_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestAMigrationsWritesAreInTheMarkTakenRightAfterIt(t *testing.T) {
	ctx := context.Background()
	pool := testdatabase.NewUnmigrated(t)

	// Migrating a new database writes rows: the pipeline's phases, the
	// profile's and the criteria's single rows.
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	mark, err := store.ReadWriteMark(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if phases := mark.Tables["pipeline_phases"]; phases.Inserted == 0 {
		t.Fatalf("the mark holds no pipeline_phases inserts (%+v): the migrations' writes weren't flushed before it", phases)
	}

	// Long enough for any count the migrations held back to show.
	time.Sleep(2 * time.Second)
	count, err := store.CountWrites(ctx, pool, mark)
	if err != nil {
		t.Fatal(err)
	}
	if !count.IsNothingLost() {
		t.Fatalf("count after migrating = %+v (%v), want nothing lost", count, count.Lost())
	}
	if got := count.Describe(time.UTC); got != "Nothing was lost." {
		t.Fatalf("Describe = %q", got)
	}
}

func TestWritesAfterTheMarkAreCountedPerTableInTheOwnersWords(t *testing.T) {
	ctx := context.Background()
	pool := testdatabase.New(t)
	mark := readSettledMark(t, pool)

	writeAndFlush(t, pool,
		`INSERT INTO companies (name, domain) VALUES ('Acme', 'acme.example')`,
		`UPDATE companies SET summary = 'Makes anvils' WHERE domain = 'acme.example'`,
		`INSERT INTO updates (kind, title) VALUES ('reply', 'Acme replied'), ('reply', 'Initech replied')`,
		// A seen mark is bookkeeping: losing it loses nothing.
		`UPDATE updates SET seen_at = now()`,
	)
	count, err := store.CountWrites(ctx, pool, mark)
	if err != nil {
		t.Fatal(err)
	}
	if count.Uncountable != "" {
		t.Fatalf("uncountable: %s", count.Uncountable)
	}
	companies := findTableWrites(count, "companies")
	if companies.Inserted != 1 || companies.Updated != 1 || companies.Deleted != 0 {
		t.Fatalf("companies = %+v, want 1 inserted and 1 updated", companies)
	}
	updates := findTableWrites(count, "updates")
	if updates.Inserted != 2 || updates.Updated != 2 {
		t.Fatalf("updates = %+v, want 2 inserted and 2 updated", updates)
	}
	want := []string{"2 updates", "1 company added", "1 company changed"}
	if got := count.Lost(); !slices.Equal(got, want) {
		t.Fatalf("Lost = %q, want %q", got, want)
	}
	if got := count.Describe(time.UTC); got != "Lost: 2 updates, 1 company added and 1 company changed." {
		t.Fatalf("Describe = %q", got)
	}
}

func TestEveryTableHasWordsOrIsBookkeeping(t *testing.T) {
	ctx := context.Background()
	pool := testdatabase.New(t)
	rows, err := pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		_, hasWords := store.TableWordsByName[table]
		isBookkeeping := store.BookkeepingTables[table]
		switch {
		case hasWords && isBookkeeping:
			t.Errorf("%s has words and is on the bookkeeping list: pick one", table)
		case !hasWords && !isBookkeeping:
			t.Errorf("%s has no words and isn't on the bookkeeping list: say what its rows are to the owner in store.TableWordsByName, or add it to store.BookkeepingTables if losing them loses nothing", table)
		}
	}
	for table := range store.TableWordsByName {
		if !slices.Contains(tables, table) {
			t.Errorf("store.TableWordsByName names %s, which isn't a table", table)
		}
	}
	for table := range store.BookkeepingTables {
		if !slices.Contains(tables, table) {
			t.Errorf("store.BookkeepingTables names %s, which isn't a table", table)
		}
	}
}

func TestResetStatisticsCantBeCounted(t *testing.T) {
	ctx := context.Background()
	pool := testdatabase.New(t)
	writeAndFlush(t, pool, `INSERT INTO updates (kind, title) VALUES ('reply', 'Acme replied')`)
	mark, err := store.ReadWriteMark(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}

	writeAndFlush(t, pool, `SELECT pg_stat_reset()`)
	count, err := store.CountWrites(ctx, pool, mark)
	if err != nil {
		t.Fatal(err)
	}
	if count.Uncountable == "" || count.IsNothingLost() {
		t.Fatalf("count after a reset = %+v, want it uncountable", count)
	}
}

func TestADroppedOrFallingCounterCantBeCounted(t *testing.T) {
	reset := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	afterCrash := time.Date(2026, 10, 6, 3, 32, 0, 0, time.UTC)
	mark := store.WriteMark{
		At:               time.Date(2026, 10, 6, 3, 31, 0, 0, time.UTC),
		DatabaseStarted:  time.Date(2026, 10, 6, 3, 30, 0, 0, time.UTC),
		SharedStatsReset: &reset,
		Tables:           map[string]store.TableCounters{"jobs": {Inserted: 40, Updated: 12}, "changes": {Inserted: 50}},
	}
	later := func(change func(*store.WriteMark)) store.WriteMark {
		now := mark
		now.At = time.Date(2026, 10, 6, 3, 33, 0, 0, time.UTC)
		// The server the hub owns restarts Postgres cleanly, which keeps
		// the counters: a new start alone is no reason to doubt them.
		now.DatabaseStarted = time.Date(2026, 10, 6, 3, 32, 30, 0, time.UTC)
		now.Tables = map[string]store.TableCounters{"jobs": {Inserted: 43, Updated: 13}, "changes": {Inserted: 54}}
		change(&now)
		return now
	}

	clean := store.CompareWriteMarks(mark, later(func(*store.WriteMark) {}))
	if want := "Lost: 3 jobs found and 1 job changed."; clean.Describe(time.UTC) != want {
		t.Fatalf("a clean restart: %q, want %q", clean.Describe(time.UTC), want)
	}

	for name, change := range map[string]func(*store.WriteMark){
		"a crash dropped the statistics": func(now *store.WriteMark) {
			now.SharedStatsReset = &afterCrash
			now.Tables = map[string]store.TableCounters{"jobs": {Inserted: 1}}
		},
		"the database's statistics were reset": func(now *store.WriteMark) { now.StatsReset = &afterCrash },
		"a counter went down": func(now *store.WriteMark) {
			now.Tables = map[string]store.TableCounters{"jobs": {Inserted: 44, Updated: 2}, "changes": {Inserted: 55}}
		},
	} {
		count := store.CompareWriteMarks(mark, later(change))
		if count.Uncountable == "" || count.IsNothingLost() || len(count.Tables) != 0 {
			t.Errorf("%s: %+v, want it uncountable", name, count)
		}
		if want := "Anything written between 03:31 and 03:33 couldn't be counted, and was lost."; count.Describe(time.UTC) != want {
			t.Errorf("%s: %q, want %q", name, count.Describe(time.UTC), want)
		}
	}
}

func TestOnlyBookkeepingIsNothingLost(t *testing.T) {
	mark := store.WriteMark{Tables: map[string]store.TableCounters{}}
	now := store.WriteMark{Tables: map[string]store.TableCounters{
		"changes": {Inserted: 3}, "devices": {Updated: 4}, "task_runs": {Inserted: 2},
	}}
	count := store.CompareWriteMarks(mark, now)
	if !count.IsNothingLost() || len(count.Tables) != 3 {
		t.Fatalf("count = %+v, want three tables written and nothing lost", count)
	}
	now.Tables["a_table_from_the_future"] = store.TableCounters{Inserted: 2}
	if got := store.CompareWriteMarks(mark, now).Lost(); !slices.Equal(got, []string{"2 other changes"}) {
		t.Fatalf("a table without words: %q", got)
	}
}

// readSettledMark reads the counters once the writes made in setting the
// database up, such as its seeded prompts, have shown: the pool's
// connections close, which flushes their counts, and the counters stop
// moving.
func readSettledMark(t *testing.T, pool *pgxpool.Pool) store.WriteMark {
	t.Helper()
	ctx := context.Background()
	pool.Reset()
	previous, err := store.ReadWriteMark(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		time.Sleep(250 * time.Millisecond)
		mark, err := store.ReadWriteMark(ctx, pool)
		if err != nil {
			t.Fatal(err)
		}
		if len(store.CompareWriteMarks(previous, mark).Tables) == 0 {
			return mark
		}
		previous = mark
	}
	t.Fatal("the write counters kept moving")
	return store.WriteMark{}
}

// writeAndFlush runs the statements on one connection, then has its backend
// flush their write counts, which it would otherwise do only a while later.
func writeAndFlush(t *testing.T, pool *pgxpool.Pool, statements ...string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	for _, statement := range append(statements, `SELECT pg_stat_force_next_flush()`) {
		if _, err := conn.Exec(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func findTableWrites(count store.WriteCount, table string) store.TableWrites {
	for _, writes := range count.Tables {
		if writes.Table == table {
			return writes
		}
	}
	return store.TableWrites{Table: table}
}
