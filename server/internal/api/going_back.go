package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/migrations"
)

// goingBackResponse is what the Mac app reads before it offers to go back
// to an earlier version: which migration the database is at, which release
// first migrated it to each one, and each copy of the database taken before
// migrating, with what was written since, which restoring it would lose.
type goingBackResponse struct {
	// Migration is the database's, which is this server's newest: the
	// server migrates up to it at start, and won't serve one ahead.
	Migration         int64                    `json:"migration"`
	MigrationReleases []store.MigrationRelease `json:"migration_releases"`
	// Dumps are the pre-migration dumps on disk, the oldest first.
	Dumps []goingBackDump `json:"dumps"`
}

// goingBackDump is a copy of the database at Migration, taken before the
// next version migrated it, and what restoring it would lose.
type goingBackDump struct {
	Migration int64     `json:"migration"`
	DumpedAt  time.Time `json:"dumped_at"`
	// Since is the mark the version that migrated from it took before it
	// served anything: what was written from then on would be lost.
	Since *time.Time `json:"since,omitempty"`
	// Lost is what was written since, in the owner's words, the most first;
	// empty when nothing was.
	Lost []store.LostCount `json:"lost"`
	// Uncountable says why what was written since can't be counted: no
	// mark, or counters reset or dropped since. All of it would be lost.
	Uncountable string `json:"uncountable,omitempty"`
}

// goingBackReader is what the route reads from the store.
type goingBackReader interface {
	ListMigrationReleases(ctx context.Context) ([]store.MigrationRelease, error)
	CountWritesSince(ctx context.Context, mark store.WriteMark) (store.WriteCount, error)
}

// noMark is why a dump without its mark can't be counted against: the
// version that migrated from it never marked where its writes began.
const noMark = "nothing marked when the version after it began writing"

// RegisterGoingBackRoutes adds the owner-only GET /v1/version/going-back,
// which counts what going back to each earlier version would lose, from the
// dumps in backups.
func RegisterGoingBackRoutes(routes *http.ServeMux, hub goingBackReader, backups string, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/version/going-back", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, err := readGoingBack(r.Context(), hub, backups)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response)
	})))
}

func readGoingBack(ctx context.Context, hub goingBackReader, backups string) (goingBackResponse, error) {
	releases, err := hub.ListMigrationReleases(ctx)
	if err != nil {
		return goingBackResponse{}, err
	}
	dumps, err := databasebackup.ListPreMigrationDumps(backups)
	if err != nil {
		return goingBackResponse{}, err
	}
	response := goingBackResponse{
		Migration: migrations.Newest(), MigrationReleases: releases, Dumps: make([]goingBackDump, 0, len(dumps)),
	}
	if response.MigrationReleases == nil {
		response.MigrationReleases = []store.MigrationRelease{}
	}
	for _, dump := range dumps {
		counted := goingBackDump{Migration: dump.Migration, DumpedAt: dump.DumpedAt, Lost: []store.LostCount{}}
		data, err := os.ReadFile(databasebackup.MarkPath(dump.Path))
		switch {
		case errors.Is(err, os.ErrNotExist):
			counted.Uncountable = noMark
		case err != nil:
			return goingBackResponse{}, err
		default:
			var mark store.WriteMark
			if err := json.Unmarshal(data, &mark); err != nil {
				counted.Uncountable = noMark
				break
			}
			counted.Since = &mark.At
			count, err := hub.CountWritesSince(ctx, mark)
			if err != nil {
				return goingBackResponse{}, err
			}
			counted.Uncountable = count.Uncountable
			if count.Uncountable == "" {
				counted.Lost = count.LostCounts()
			}
		}
		response.Dumps = append(response.Dumps, counted)
	}
	return response, nil
}
