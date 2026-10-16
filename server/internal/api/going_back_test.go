package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/migrations"
)

// fakeGoingBack answers for the store: the releases recorded, and the
// counters read now.
type fakeGoingBack struct {
	releases []store.MigrationRelease
	now      store.WriteMark
}

func (fake fakeGoingBack) ListMigrationReleases(context.Context) ([]store.MigrationRelease, error) {
	return fake.releases, nil
}

func (fake fakeGoingBack) CountWritesSince(_ context.Context, mark store.WriteMark) (store.WriteCount, error) {
	return store.CompareWriteMarks(mark, fake.now), nil
}

func allowOwner(next http.Handler) http.Handler { return next }

func writeDump(t *testing.T, folder string, migration string, at time.Time, mark *store.WriteMark) {
	t.Helper()
	dump := filepath.Join(folder, "hub-pre-migration-"+migration+".dump")
	if err := os.WriteFile(dump, []byte("dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dump, at, at); err != nil {
		t.Fatal(err)
	}
	if mark == nil {
		return
	}
	data, err := json.Marshal(mark)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "hub-pre-migration-"+migration+".counts.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGoingBackCountsWhatEachDumpWouldLoseInTheOwnersWords(t *testing.T) {
	backups := t.TempDir()
	dumpedAt := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	markedAt := dumpedAt.Add(2 * time.Minute)
	reset := dumpedAt.Add(-time.Hour)
	mark := store.WriteMark{At: markedAt, StatsReset: &reset, Tables: map[string]store.TableCounters{"jobs": {Inserted: 10}, "applications": {Updated: 4}}}
	// The oldest dump's mark is from before a statistics reset.
	writeDump(t, backups, "85", dumpedAt.Add(-48*time.Hour), &store.WriteMark{At: dumpedAt.Add(-48 * time.Hour), Tables: map[string]store.TableCounters{}})
	writeDump(t, backups, "86", dumpedAt.Add(-24*time.Hour), nil)
	writeDump(t, backups, "88", dumpedAt, &mark)
	now := store.WriteMark{At: dumpedAt.Add(5 * time.Hour), StatsReset: &reset, Tables: map[string]store.TableCounters{
		"jobs": {Inserted: 13}, "applications": {Updated: 5}, "updates": {Inserted: 4}, "changes": {Inserted: 8},
	}}
	fake := fakeGoingBack{releases: []store.MigrationRelease{{Migration: 85, Release: "0.1.240"}, {Migration: 88, Release: "0.1.247"}}, now: now}

	routes := http.NewServeMux()
	RegisterGoingBackRoutes(routes, fake, backups, allowOwner)
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/version/going-back", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
	var body goingBackResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Migration != migrations.Newest() || !slices.Equal(body.MigrationReleases, fake.releases) {
		t.Fatalf("migration %d, releases %+v", body.Migration, body.MigrationReleases)
	}
	if len(body.Dumps) != 3 {
		t.Fatalf("dumps = %+v", body.Dumps)
	}
	if oldest := body.Dumps[0]; oldest.Migration != 85 || oldest.Uncountable != "the database's statistics were reset" || len(oldest.Lost) != 0 {
		t.Fatalf("dump of 85 = %+v", oldest)
	}
	if unmarked := body.Dumps[1]; unmarked.Migration != 86 || unmarked.Uncountable != noMark || unmarked.Since != nil {
		t.Fatalf("dump of 86 = %+v", unmarked)
	}
	newest := body.Dumps[2]
	want := []store.LostCount{{Rows: 4, Words: "updates"}, {Rows: 3, Words: "jobs found"}, {Rows: 1, Words: "card moved"}}
	if newest.Migration != 88 || !newest.DumpedAt.Equal(dumpedAt) || newest.Since == nil || !newest.Since.Equal(markedAt) || newest.Uncountable != "" || !slices.Equal(newest.Lost, want) {
		t.Fatalf("dump of 88 = %+v, want lost %+v", newest, want)
	}
}

func TestGoingBackWithoutBackupsListsNoDumps(t *testing.T) {
	routes := http.NewServeMux()
	RegisterGoingBackRoutes(routes, fakeGoingBack{}, filepath.Join(t.TempDir(), "missing"), allowOwner)
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/version/going-back", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); body != `{"migration":`+jsonNumber(migrations.Newest())+`,"migration_releases":[],"dumps":[]}`+"\n" {
		t.Fatalf("body = %s", body)
	}
}

func jsonNumber(number int64) string {
	data, _ := json.Marshal(number)
	return string(data)
}
