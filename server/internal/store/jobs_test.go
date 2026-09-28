package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var hubSystem = store.Actor{Kind: store.ActorSystem}

func createBoard(t *testing.T, hub *store.Store) store.JobBoard {
	t.Helper()
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	board, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: company.ID, Provider: "lever", BoardToken: "acme", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	return board
}

func posting(externalID, title string) store.JobPosting {
	return store.JobPosting{ExternalID: externalID, Title: title, URL: "https://jobs.lever.co/acme/" + externalID}
}

func jobOperations(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT operation || ':' || actor_kind FROM changes WHERE entity_type = 'job' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return operations
}

func TestSyncingABoardTracksEachPostingsLifecycle(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	board := createBoard(t, hub)
	firstSync := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)

	result, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{posting("a", "Frontend Engineer"), posting("b", "Backend Engineer")}, firstSync)
	if err != nil || result != (store.BoardSyncResult{Created: 2, Seen: 2}) {
		t.Fatalf("first sync = %+v, %v", result, err)
	}

	secondSync := firstSync.Add(time.Hour)
	result, err = hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{posting("a", "Senior Frontend Engineer")}, secondSync)
	if err != nil || result != (store.BoardSyncResult{Closed: 1, Seen: 1}) {
		t.Fatalf("second sync = %+v, %v", result, err)
	}
	var title string
	var lastSeen time.Time
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT title, last_seen_at, (SELECT count(*) FROM jobs) FROM jobs WHERE external_id = 'a'`).Scan(&title, &lastSeen, &jobs); err != nil {
		t.Fatal(err)
	}
	if title != "Senior Frontend Engineer" || !lastSeen.Equal(secondSync) || jobs != 2 {
		t.Fatalf("job a: title=%q last_seen=%v jobs=%d", title, lastSeen, jobs)
	}

	result, err = hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{posting("a", "Senior Frontend Engineer"), posting("b", "Backend Engineer")}, secondSync.Add(time.Hour))
	if err != nil || result != (store.BoardSyncResult{Reopened: 1, Seen: 2}) {
		t.Fatalf("third sync = %+v, %v", result, err)
	}

	operations := jobOperations(t, pool)
	want := []string{"create:system", "create:system", "close:system", "reopen:system"}
	if len(operations) != len(want) {
		t.Fatalf("job changes = %v, want %v", operations, want)
	}
	for index := range want {
		if operations[index] != want[index] {
			t.Fatalf("job changes = %v, want %v", operations, want)
		}
	}
}

func TestAnEmptyBoardClosesEveryOpenJob(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	board := createBoard(t, hub)
	ctx := context.Background()
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, []store.JobPosting{posting("a", "Engineer")}, time.Now()); err != nil {
		t.Fatal(err)
	}

	result, err := hub.SyncBoardJobs(ctx, hubSystem, board, nil, time.Now())
	if err != nil || result.Closed != 1 {
		t.Fatalf("sync = %+v, %v", result, err)
	}
}

func TestAJobAddedByHandNeedsATitleAndAURLAndIsStoredOnce(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()

	for _, input := range []store.ManualJobInput{{Title: "", URL: "https://acme.com/jobs/1"}, {Title: "Engineer", URL: "acme.com/jobs/1"}} {
		if _, _, err := hub.AddManualJob(ctx, owner, input); err == nil {
			t.Errorf("input %+v was accepted", input)
		}
	}
	first, created, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/jobs/1"})
	if err != nil || !created || first.Source != store.JobSourceManual || first.CompanyID != nil {
		t.Fatalf("add = %+v, created=%v, err=%v", first, created, err)
	}
	again, created, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer (again)", URL: "https://acme.com/jobs/1"})
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("second add = %+v, created=%v, err=%v", again, created, err)
	}
	if operations := jobOperations(t, pool); len(operations) != 1 || operations[0] != "create:owner" {
		t.Fatalf("job changes = %v", operations)
	}
}

func TestAJobAddedByHandForAnUnknownCompanyFails(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	unknown := createBoard(t, hub).ID

	_, _, err := hub.AddManualJob(context.Background(), owner, store.ManualJobInput{CompanyID: &unknown, Title: "Engineer", URL: "https://acme.com/jobs/1"})
	if !errors.Is(err, store.ErrCompanyNotFound) {
		t.Fatalf("err = %v, want ErrCompanyNotFound", err)
	}
}
