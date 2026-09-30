package store_test

import (
	"context"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestDefaultRoutesAreSeededOnceAndNeverOverwriteAChoice(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if err := hub.EnsureDefaultTaskRoutes(ctx, "", "qwen", store.RoutedTaskKinds); err != nil {
		t.Fatal(err)
	}
	if routes, _ := hub.ListTaskRoutes(ctx); len(routes) != 0 {
		t.Fatalf("no model server in the settings still made routes: %+v", routes)
	}

	if err := hub.EnsureDefaultTaskRoutes(ctx, "http://localhost:1234/v1/", "qwen/qwen3.5-9b", store.RoutedTaskKinds); err != nil {
		t.Fatal(err)
	}
	routes, _ := hub.ListTaskRoutes(ctx)
	providers, _ := hub.ListModelProviders(ctx)
	if len(routes) != 3 || len(providers) != 1 || providers[0].Name != store.DefaultModelProviderName || providers[0].BaseURL != "http://localhost:1234/v1" {
		t.Fatalf("routes = %+v, providers = %+v", routes, providers)
	}

	hosted, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "Hosted", BaseURL: "https://example.com/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTaskRoute(ctx, owner, store.AgentPromptKindJobFacts, store.TaskRouteInput{ProviderID: hosted.ID, Model: "big"}); err != nil {
		t.Fatal(err)
	}
	if err := hub.EnsureDefaultTaskRoutes(ctx, "http://localhost:1234/v1", "qwen/qwen3.5-9b", store.RoutedTaskKinds); err != nil {
		t.Fatal(err)
	}
	route, _ := hub.GetTaskRoute(ctx, store.AgentPromptKindJobFacts)
	providers, _ = hub.ListModelProviders(ctx)
	if route.ProviderID != hosted.ID || route.Model != "big" || len(providers) != 2 {
		t.Fatalf("a restart overwrote the chosen route: %+v, providers %d", route, len(providers))
	}
}

func TestModelWorkStartsPausedAndRemembersItsPause(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if paused, err := hub.GetModelWorkPaused(ctx); err != nil || !paused {
		t.Fatalf("a new hub: paused = %v, %v", paused, err)
	}
	if err := hub.SetModelWorkPaused(ctx, owner, false); err != nil {
		t.Fatal(err)
	}
	if paused, _ := hub.GetModelWorkPaused(ctx); paused {
		t.Fatal("the resume didn't stick")
	}
}
