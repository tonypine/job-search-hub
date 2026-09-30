package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestABriefIsKeptPerTierAndGoesStaleWithTheKnowledgeBase(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Product Engineer", URL: "https://acme.com/jobs/1"})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: "case", Title: "Shipped the scheduler", Source: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	hash, err := hub.GetKnowledgeHash(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if brief, err := hub.GetJobBrief(ctx, job.ID); err != nil || brief != nil {
		t.Fatalf("before any brief: %+v, %v", brief, err)
	}
	pre := store.JobBrief{
		JobID: job.ID, Tier: store.JobBriefTierPre, PromptID: prompt.ID, Model: "local", Match: "possible", Reason: "Close.",
		Strengths:     []store.JobBriefPoint{{Point: "Scheduling", EntryIDs: []uuid.UUID{entry.ID, uuid.New()}}},
		KnowledgeHash: hash,
	}
	if err := hub.SaveJobBrief(ctx, pre); err != nil {
		t.Fatal(err)
	}
	brief, err := hub.GetJobBrief(ctx, job.ID)
	if err != nil || brief == nil || brief.Match != "possible" || brief.IsStale || len(brief.Weaknesses) != 0 {
		t.Fatalf("pre-brief = %+v, %v", brief, err)
	}
	if len(brief.CitedEntries) != 1 || brief.CitedEntries[0].Title != "Shipped the scheduler" {
		t.Errorf("cited = %+v, want the one entry that exists", brief.CitedEntries)
	}
	if items, _, _ := hub.ListJobs(ctx, store.JobFilter{}); items[0].Match == nil || *items[0].Match != "possible" {
		t.Errorf("the list's match = %v", items[0].Match)
	}

	full := pre
	full.Tier, full.Model, full.Match = store.JobBriefTierFull, "claude", "strong"
	if err := hub.SaveJobBrief(ctx, full); err != nil {
		t.Fatal(err)
	}
	if brief, _ := hub.GetJobBrief(ctx, job.ID); brief.Tier != store.JobBriefTierFull || brief.Match != "strong" {
		t.Errorf("with both tiers: %+v, want the full brief", brief)
	}

	if _, err := hub.SaveProfileEntry(ctx, owner, &entry.ID, store.ProfileEntryInput{Kind: "case", Title: "Shipped the scheduler", Outcome: "Cut no-shows", Source: "owner"}); err != nil {
		t.Fatal(err)
	}
	if brief, _ := hub.GetJobBrief(ctx, job.ID); !brief.IsStale {
		t.Error("the brief isn't stale after its knowledge base changed")
	}
	if err := hub.SaveJobBrief(ctx, store.JobBrief{JobID: uuid.New(), Tier: "pre", PromptID: prompt.ID, Model: "x", Match: "strong", Reason: "x"}); err == nil {
		t.Error("expected an error for an unknown job")
	}
}
