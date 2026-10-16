package store_test

import (
	"context"
	"errors"
	"strings"
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
	if len(routes) != len(store.RoutedTaskKinds) || len(providers) != 1 || providers[0].Name != store.DefaultModelProviderName || providers[0].BaseURL != "http://localhost:1234/v1" {
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

func TestTheClaudeModelSettingBecomesTheRoutesOfFullBriefsAndCVs(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	// A provider already named Claude, such as a proxy, keeps its name.
	if _, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "Claude", BaseURL: "https://proxy.example.com/v1"}); err != nil {
		t.Fatal(err)
	}
	claude, err := hub.EnsureClaudeTaskRoutes(ctx, "sonnet", store.ClaudeTaskKinds)
	if err != nil || claude.Kind != store.ModelProviderKindClaudeSubscription || claude.Type != store.ModelProviderTypeClaudeSubscription ||
		claude.Name != "Claude subscription" || claude.BaseURL != "" {
		t.Fatalf("the Claude provider: %+v, %v", claude, err)
	}
	for _, kind := range store.ClaudeTaskKinds {
		if route, err := hub.GetTaskRoute(ctx, kind); err != nil || route.ProviderID != claude.ID || route.Model != "sonnet" {
			t.Fatalf("%s: %+v, %v", kind, route, err)
		}
	}

	// The owner routes full briefs to this Mac; a restart with another
	// setting changes neither that route nor the provider.
	local, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "Local", BaseURL: "http://localhost:1234/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTaskRoute(ctx, owner, store.RunKindJobBriefFull, store.TaskRouteInput{ProviderID: local.ID, Model: "qwen",
		FallbackProviderID: &claude.ID, FallbackModel: "sonnet"}); err != nil {
		t.Fatal(err)
	}
	again, err := hub.EnsureClaudeTaskRoutes(ctx, "opus", store.ClaudeTaskKinds)
	if err != nil || again.ID != claude.ID {
		t.Fatalf("a restart: %+v, %v", again, err)
	}
	if route, _ := hub.GetTaskRoute(ctx, store.RunKindJobBriefFull); route.ProviderID != local.ID || route.Model != "qwen" {
		t.Fatalf("a restart overwrote the chosen route: %+v", route)
	}
	if route, _ := hub.GetTaskRoute(ctx, store.AgentPromptKindJobCV); route.Model != "sonnet" {
		t.Fatalf("a restart changed a seeded route: %+v", route)
	}

	// Each route change is recorded with who made it.
	rows, err := pool.Query(ctx, `SELECT actor_kind, operation FROM changes WHERE entity_type = 'task_route' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var changes []string
	for rows.Next() {
		var actor, operation string
		if err := rows.Scan(&actor, &operation); err != nil {
			t.Fatal(err)
		}
		changes = append(changes, actor+" "+operation)
	}
	want := []string{"system route job_brief_full", "system route job_cv", "owner route job_brief_full"}
	if strings.Join(changes, ", ") != strings.Join(want, ", ") {
		t.Fatalf("changes = %v, want %v", changes, want)
	}
}

func TestAnAgentKindRunsOnlyOnTheClaudeSubscription(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	local, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "Local", BaseURL: "http://localhost:1234/v1"})
	if err != nil {
		t.Fatal(err)
	}
	claude, err := hub.EnsureClaudeTaskRoutes(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{store.AgentRunKindCompanyTriage, store.AgentRunKindJobFinder, store.AgentRunKindJobFix, store.AgentRunKindProfileSeed} {
		if _, err := hub.SaveTaskRoute(ctx, owner, kind, store.TaskRouteInput{ProviderID: local.ID, Model: "qwen"}); !errors.Is(err, store.ErrAgentRunsOnClaude) {
			t.Errorf("%s on a local model: %v", kind, err)
		}
		if _, err := hub.SaveTaskRoute(ctx, owner, kind, store.TaskRouteInput{ProviderID: claude.ID, Model: "opus"}); err != nil {
			t.Errorf("%s on Claude: %v", kind, err)
		}
	}
	if _, err := hub.SaveTaskRoute(ctx, owner, "no_such_kind", store.TaskRouteInput{ProviderID: claude.ID, Model: "opus"}); err == nil {
		t.Error("a kind the catalog lacks got a route")
	}
}

func TestAProviderAnAgentRunsOnStaysClaude(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	claude, err := hub.EnsureClaudeTaskRoutes(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTaskRoute(ctx, owner, store.AgentRunKindJobFix, store.TaskRouteInput{ProviderID: claude.ID, Model: "opus"}); err != nil {
		t.Fatal(err)
	}
	local := store.ModelProviderInput{Kind: store.ModelProviderKindOpenAICompatible, Name: claude.Name, BaseURL: "http://localhost:1234/v1"}
	if _, err := hub.SaveModelProvider(ctx, owner, &claude.ID, local); !errors.Is(err, store.ErrAgentRunsOnClaude) {
		t.Fatalf("turning an agent's provider into a local model: %v", err)
	}
	if got, err := hub.GetModelProvider(ctx, claude.ID); err != nil || got.Kind != store.ModelProviderKindClaudeSubscription {
		t.Fatalf("provider after the refused change = %+v, %v", got, err)
	}
	if _, err := hub.SaveModelProvider(ctx, owner, &claude.ID, store.ModelProviderInput{Kind: store.ModelProviderKindClaudeSubscription, Name: "Claude Max"}); err != nil {
		t.Fatalf("renaming an agent's provider: %v", err)
	}
	other, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Kind: store.ModelProviderKindClaudeSubscription, Name: "Other Claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTaskRoute(ctx, owner, store.AgentRunKindJobFix, store.TaskRouteInput{ProviderID: other.ID, Model: "opus"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveModelProvider(ctx, owner, &claude.ID, local); err != nil {
		t.Fatalf("changing a provider no agent runs on: %v", err)
	}
}
