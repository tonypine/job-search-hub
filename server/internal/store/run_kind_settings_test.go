package store_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/migrations"
)

// runKindPausesMigration is the migration that split the one pause by kind.
const runKindPausesMigration = 88

// kindsWhenPausesSplit are the kinds the hub started by itself then, which
// the migration seeds.
var kindsWhenPausesSplit = []string{
	store.AgentPromptKindJobFacts, store.AgentPromptKindJobBrief, store.RunKindJobBriefFull, store.AgentPromptKindJobCV,
	store.AgentPromptKindRecruiterScreen, store.AgentPromptKindInterviewPrep, store.AgentPromptKindMailTriage,
	store.AgentPromptKindLinkedInConversation, store.AgentPromptKindMarketGaps, store.AgentPromptKindHiringThread,
}

func TestANewHubStartsWithEveryAutomaticKindPaused(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	pauses, err := hub.GetRunKindPauses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if kinds := slices.Sorted(maps.Keys(pauses)); !slices.Equal(kinds, slices.Sorted(slices.Values(store.ListAutomaticRunKindIDs()))) {
		t.Fatalf("kinds = %v", kinds)
	}
	for kind, paused := range pauses {
		if !paused {
			t.Errorf("%s isn't paused on a new hub", kind)
		}
	}
	if _, listed := pauses[store.AgentRunKindJobFinder]; listed {
		t.Error("find jobs runs only when asked, yet has a pause")
	}
}

func TestEachKindsPauseSurvivesARestartAndIsRecordedWithItsActor(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	if err := hub.SetRunKindsPaused(ctx, owner, []string{store.AgentPromptKindJobFacts, store.AgentPromptKindMailTriage}, false); err != nil {
		t.Fatal(err)
	}
	if err := hub.SetRunKindsPaused(ctx, store.Actor{Kind: store.ActorSystem}, []string{store.AgentPromptKindMailTriage}, true); err != nil {
		t.Fatal(err)
	}

	restarted := store.New(pool)
	pauses, err := restarted.GetRunKindPauses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pauses[store.AgentPromptKindJobFacts] || !pauses[store.AgentPromptKindMailTriage] || !pauses[store.AgentPromptKindJobBrief] {
		t.Fatalf("after a restart: %v", pauses)
	}

	rows, err := pool.Query(ctx, `SELECT actor_kind, operation, after->>'kind' FROM changes WHERE entity_type = 'model_work' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var changes []string
	for rows.Next() {
		var actor, operation, kind string
		if err := rows.Scan(&actor, &operation, &kind); err != nil {
			t.Fatal(err)
		}
		changes = append(changes, actor+" "+operation+" "+kind)
	}
	want := []string{"owner resume job_facts", "owner resume mail_triage", "system pause mail_triage"}
	if !slices.Equal(changes, want) {
		t.Fatalf("changes = %v, want %v", changes, want)
	}
}

func TestAKindOnlyTheOwnerStartsRefusesAPause(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if err := hub.SetRunKindsPaused(ctx, owner, []string{store.AgentPromptKindJobFacts, store.AgentRunKindJobFinder}, false); !errors.Is(err, store.ErrRunKindNotPausable) {
		t.Fatalf("find jobs: %v", err)
	}
	if err := hub.SetRunKindsPaused(ctx, owner, []string{"no_such_kind"}, true); !errors.Is(err, store.ErrRunKindNotFound) {
		t.Fatalf("an unknown kind: %v", err)
	}
	if pauses, _ := hub.GetRunKindPauses(ctx); !pauses[store.AgentPromptKindJobFacts] {
		t.Fatal("a refused change still resumed job facts")
	}
}

func TestEveryKindStartsWithTheOnePauseItReplaces(t *testing.T) {
	for _, paused := range []bool{true, false} {
		pool := testdatabase.New(t)
		hub := store.New(pool)
		ctx := context.Background()
		database := stdlib.OpenDBFromPool(pool)
		provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.DownTo(ctx, runKindPausesMigration-1); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE model_work_settings SET paused = $1`, paused); err != nil {
			t.Fatal(err)
		}
		if _, err := provider.UpTo(ctx, runKindPausesMigration); err != nil {
			t.Fatal(err)
		}
		database.Close()
		pauses, err := hub.GetRunKindPauses(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range kindsWhenPausesSplit {
			if pauses[kind] != paused {
				t.Errorf("with the one pause %v, %s paused = %v", paused, kind, pauses[kind])
			}
		}
	}
}
