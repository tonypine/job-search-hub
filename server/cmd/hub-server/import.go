package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

// parseImportArguments reads import's arguments: the URL of the Postgres to
// import from, and --replace, before or after it.
func parseImportArguments(arguments []string) (source string, replace, ok bool) {
	for _, argument := range arguments {
		switch {
		case argument == "--replace":
			replace = true
		case strings.HasPrefix(argument, "-") || source != "":
			return "", false, false
		default:
			source = argument
		}
	}
	if source == "" {
		return "", false, false
	}
	return source, replace, true
}

// importCheck checks the database imported from source before it is kept;
// checkImported in production.
type importCheck func(ctx context.Context, source, imported string, out io.Writer) error

// importDatabase moves the hub's data from the Postgres at source into a new
// cluster in dir, run by the newest engine in engines. It dumps source into
// backups with the engine's pg_dump and restores the dump as a restore does,
// keeping the new cluster only if check passes: each of its tables holds as
// many rows as in source. Without replace, it refuses when the cluster in dir
// holds data already.
func importDatabase(ctx context.Context, engines, dir, backups, source string, replace bool, check importCheck, out io.Writer) error {
	engine, err := postgresprocess.NewestEngine(engines)
	if err != nil {
		return err
	}
	owner, err := postgresprocess.Own(postgresprocess.Settings{Engine: engine, Dir: dir})
	if errors.Is(err, postgresprocess.ErrLocked) {
		return fmt.Errorf("%w; stop the hub in Settings › Server first", err)
	}
	if err != nil {
		return err
	}
	defer owner.Release()
	if !replace {
		if err := refuseHeldData(ctx, owner); err != nil {
			return err
		}
	}

	fmt.Fprintln(out, "Dumping the database to import")
	dump, err := databasebackup.DumpForImport(ctx, filepath.Join(engine, "bin", "pg_dump"), source, backups, time.Now())
	if err != nil {
		return fmt.Errorf("dump the database to import: %w", err)
	}
	replaced, err := owner.Restore(ctx, dump, func(ctx context.Context, imported string) error {
		return check(ctx, source, imported, out)
	}, out)
	if err != nil {
		return fmt.Errorf("the import failed, and the database the server owns is as it was: %w", err)
	}
	fmt.Fprintf(out, "Imported the database into %s. Its dump is kept in %s.\n", dir, dump)
	if replaced != "" {
		fmt.Fprintf(out, "The database it replaced is kept in %s; delete that folder once the hub runs well.\n", replaced)
	}
	return nil
}

// refuseHeldData fails when the cluster the owner holds has a row in any
// table: a database the hub ran on, even once, which an import would replace.
func refuseHeldData(ctx context.Context, owner *postgresprocess.Owner) error {
	var held int64
	found, err := owner.Inspect(ctx, func(ctx context.Context, databaseURL string) error {
		counts, err := countRows(ctx, databaseURL)
		for _, count := range counts {
			held += count
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("check whether the database the server owns holds data: %w", err)
	}
	if found && held > 0 {
		return fmt.Errorf("the database the server owns already holds data (%d rows); to replace it with the import, run it again with --replace, which keeps the current one in a folder beside it", held)
	}
	return nil
}

// checkImported counts each table's rows in source and in the imported
// database, prints both, and fails on any difference. Then it checks the
// imported database is the hub's and migrates it, as a restore does.
func checkImported(ctx context.Context, source, imported string, out io.Writer) error {
	sourceCounts, err := countRows(ctx, source)
	if err != nil {
		return fmt.Errorf("count the rows in the database to import: %w", err)
	}
	importedCounts, err := countRows(ctx, imported)
	if err != nil {
		return fmt.Errorf("count the rows in the imported database: %w", err)
	}
	if differing := printRowCounts(out, sourceCounts, importedCounts); len(differing) > 0 {
		return fmt.Errorf("the imported database's row counts differ from the source's in %s", strings.Join(differing, ", "))
	}
	return migrateRestored(ctx, imported)
}

// countRows counts the rows of each table in the database at databaseURL, in
// one snapshot. A table outside the public schema is named with its schema.
func countRows(ctx context.Context, databaseURL string) (map[string]int64, error) {
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	defer connection.Close(context.Background())
	transaction, err := connection.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback(context.Background())
	rows, err := transaction.Query(ctx, `
		SELECT namespace.nspname, class.relname FROM pg_class AS class
		JOIN pg_namespace AS namespace ON namespace.oid = class.relnamespace
		WHERE class.relkind IN ('r', 'p') AND class.relpersistence <> 't'
			AND namespace.nspname <> 'information_schema' AND namespace.nspname NOT LIKE 'pg\_%'`)
	if err != nil {
		return nil, err
	}
	tables, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pgx.Identifier, error) {
		var schema, name string
		err := row.Scan(&schema, &name)
		return pgx.Identifier{schema, name}, err
	})
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := transaction.QueryRow(ctx, "SELECT count(*) FROM "+table.Sanitize()).Scan(&count); err != nil {
			return nil, fmt.Errorf("count the rows in %s: %w", table.Sanitize(), err)
		}
		name := table[1]
		if table[0] != "public" {
			name = table[0] + "." + table[1]
		}
		counts[name] = count
	}
	return counts, nil
}

// printRowCounts prints each table's rows in source and imported, by name,
// and returns the tables where they differ, a table missing from one
// included.
func printRowCounts(out io.Writer, source, imported map[string]int64) (differing []string) {
	names := slices.Collect(maps.Keys(source))
	for name := range imported {
		if _, found := source[name]; !found {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	var aligned bytes.Buffer
	table := tabwriter.NewWriter(&aligned, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "table\tsource\timported\t")
	for _, name := range names {
		sourceCount, inSource := source[name]
		importedCount, inImported := imported[name]
		line := name + "\t" + formatCount(sourceCount, inSource) + "\t" + formatCount(importedCount, inImported) + "\t"
		if !inSource || !inImported || sourceCount != importedCount {
			differing = append(differing, name)
			line += "differs"
		}
		fmt.Fprintln(table, line)
	}
	table.Flush()
	for line := range strings.Lines(aligned.String()) {
		fmt.Fprintln(out, strings.TrimRight(line, " \n"))
	}
	return differing
}

func formatCount(count int64, found bool) string {
	if !found {
		return "-"
	}
	return strconv.FormatInt(count, 10)
}
