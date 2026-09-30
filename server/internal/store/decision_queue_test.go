package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestTheQueueHoldsBriefedUndecidedJobsBestMatchFirst(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	hash, _ := hub.GetKnowledgeHash(ctx)
	ids := map[string]store.Job{}
	for _, title := range []string{"Stretch", "Strong", "Unbriefed", "Possible", "Later", "Pursued"} {
		job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: title, URL: "https://acme.com/" + title})
		if err != nil {
			t.Fatal(err)
		}
		ids[title] = job
		match := map[string]string{"Stretch": "stretch", "Strong": "strong", "Possible": "possible", "Later": "strong", "Pursued": "strong"}[title]
		if match != "" {
			if err := hub.SaveJobBrief(ctx, store.JobBrief{JobID: job.ID, Tier: store.JobBriefTierPre, PromptID: prompt.ID, Model: "m", Match: match, Reason: title + " reason", KnowledgeHash: hash}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := hub.DecideJob(ctx, owner, ids["Later"].ID, store.JobDecisionLater, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.DecideJob(ctx, owner, ids["Pursued"].ID, store.JobDecisionPursue, ""); err != nil {
		t.Fatal(err)
	}

	titles := func(laterBefore time.Time) []string {
		items, err := hub.ListDecisionQueue(ctx, laterBefore)
		if err != nil {
			t.Fatal(err)
		}
		var listed []string
		for _, item := range items {
			listed = append(listed, item.Job.Title)
		}
		return listed
	}
	today := time.Now().Add(-time.Minute)
	if got := titles(today); len(got) != 3 || got[0] != "Strong" || got[1] != "Possible" || got[2] != "Stretch" {
		t.Errorf("today's queue = %v, want Strong, Possible, Stretch", got)
	}
	if got := titles(time.Now().Add(24 * time.Hour)); len(got) != 4 || got[3] != "Later" {
		t.Errorf("the next day's queue = %v, want the job left for later last", got)
	}
	items, _ := hub.ListDecisionQueue(ctx, today)
	if items[0].Reason != "Strong reason" || items[0].BriefTier != store.JobBriefTierPre || items[0].Decision != nil {
		t.Errorf("first item = %+v", items[0])
	}
}
