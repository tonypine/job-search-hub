package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestTheWatchListKeepsOneActiveEntryPerCompany(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Stripe", Domain: "stripe.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	firstSince, added, err := hub.AddToWatchList(ctx, owner, company.ID)
	if err != nil || !added {
		t.Fatalf("first add: added=%v err=%v", added, err)
	}
	againSince, addedAgain, err := hub.AddToWatchList(ctx, owner, company.ID)
	if err != nil || addedAgain || !againSince.Equal(firstSince) {
		t.Fatalf("second add: added=%v since=%v err=%v, want the first entry", addedAgain, againSince, err)
	}

	watched, err := hub.ListWatchList(ctx)
	if err != nil || len(watched) != 1 || watched[0].Company.ID != company.ID {
		t.Fatalf("watch list = %+v, err = %v", watched, err)
	}
	var activeEntries int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM watch_list_entries WHERE removed_at IS NULL`).Scan(&activeEntries); err != nil || activeEntries != 1 {
		t.Fatalf("active entries = %d, err = %v", activeEntries, err)
	}
}

func TestRemovingFromTheWatchListKeepsTheEntryAndRecordsIt(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Clio", Domain: "clio.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := hub.AddToWatchList(ctx, owner, company.ID); err != nil {
		t.Fatalf("add: %v", err)
	}

	removed, err := hub.RemoveFromWatchList(ctx, owner, company.ID)
	if err != nil || !removed {
		t.Fatalf("remove: removed=%v err=%v", removed, err)
	}
	if removedAgain, err := hub.RemoveFromWatchList(ctx, owner, company.ID); err != nil || removedAgain {
		t.Fatalf("second remove: removed=%v err=%v, want false", removedAgain, err)
	}

	if watched, err := hub.ListWatchList(ctx); err != nil || len(watched) != 0 {
		t.Fatalf("watch list = %+v, err = %v, want empty", watched, err)
	}
	if since, err := hub.GetWatchedSince(ctx, company.ID); err != nil || since != nil {
		t.Fatalf("watched since = %v, err = %v, want nil", since, err)
	}
	var entries, removedEntries, watchListChanges int
	err = pool.QueryRow(ctx, `
		SELECT count(*), count(removed_at),
		       (SELECT count(*) FROM changes WHERE entity_type = 'watch_list_entry')
		FROM watch_list_entries`).Scan(&entries, &removedEntries, &watchListChanges)
	if err != nil || entries != 1 || removedEntries != 1 || watchListChanges != 2 {
		t.Fatalf("entries=%d removed=%d changes=%d err=%v, want 1, 1, 2", entries, removedEntries, watchListChanges, err)
	}
}

func TestAddingAnUnknownCompanyToTheWatchListFails(t *testing.T) {
	hub := store.New(testdatabase.New(t))

	_, _, err := hub.AddToWatchList(context.Background(), owner, uuid.New())
	if !errors.Is(err, store.ErrCompanyNotFound) {
		t.Fatalf("err = %v, want ErrCompanyNotFound", err)
	}
}
