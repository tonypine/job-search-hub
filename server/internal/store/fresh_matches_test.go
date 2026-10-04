package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestAFreshStrongMatchIsListedUntilTold(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	board := createBoard(t, hub)
	now := time.Now().Truncate(time.Second)
	publishedAgo := func(externalID string, age time.Duration) store.JobPosting {
		published := now.Add(-age)
		posting := posting(externalID, externalID)
		posting.PublishedAt = &published
		return posting
	}
	postings := []store.JobPosting{
		publishedAgo("fresh", 5*time.Hour),
		publishedAgo("old", 5*24*time.Hour),
		posting("undated", "undated"),
		publishedAgo("possible", time.Hour),
		publishedAgo("overruled", time.Hour),
		publishedAgo("pursued", time.Hour),
		publishedAgo("later", time.Hour),
		publishedAgo("skipped", time.Hour),
		publishedAgo("closed", time.Hour),
	}
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, postings, now); err != nil {
		t.Fatal(err)
	}
	jobs, _, err := hub.ListJobs(ctx, store.JobFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]uuid.UUID{}
	for _, item := range jobs {
		byTitle[item.Job.Title] = item.Job.ID
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	brief := func(title, tier, match string) {
		t.Helper()
		if err := hub.SaveJobBrief(ctx, store.JobBrief{JobID: byTitle[title], Tier: tier, PromptID: prompt.ID, Model: "local", Match: match, Reason: title + " fits."}); err != nil {
			t.Fatal(err)
		}
	}
	for _, title := range []string{"fresh", "old", "undated", "overruled", "pursued", "later", "skipped", "closed"} {
		brief(title, store.JobBriefTierPre, "strong")
	}
	brief("possible", store.JobBriefTierPre, "possible")
	brief("overruled", store.JobBriefTierFull, "mismatch")
	for title, decision := range map[string]string{"pursued": store.JobDecisionPursue, "later": store.JobDecisionLater, "skipped": store.JobDecisionSkip} {
		if _, err := hub.DecideJob(ctx, owner, byTitle[title], decision, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := hub.SyncBoardJobs(ctx, hubSystem, board, postings[:len(postings)-1], now); err != nil {
		t.Fatal(err)
	}

	matches, err := hub.ListFreshMatchesToTell(ctx, now.Add(-3*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].JobTitle != "undated" || matches[1].JobTitle != "fresh" {
		t.Fatalf("matches = %+v, want undated then fresh", matches)
	}
	fresh := matches[1]
	if fresh.CompanyName == nil || *fresh.CompanyName != "Acme" || fresh.CompanyID == nil || fresh.Reason != "fresh fits." {
		t.Errorf("fresh = %+v", fresh)
	}
	if fresh.PublishedAt == nil || !fresh.PublishedAt.Equal(now.Add(-5*time.Hour)) || matches[0].PublishedAt != nil {
		t.Errorf("published at = %v and %v", fresh.PublishedAt, matches[0].PublishedAt)
	}

	if err := hub.MarkFreshMatchTold(ctx, fresh.JobID, now); err != nil {
		t.Fatal(err)
	}
	brief("fresh", store.JobBriefTierFull, "strong")
	if matches, _ := hub.ListFreshMatchesToTell(ctx, now.Add(-3*24*time.Hour)); len(matches) != 1 || matches[0].JobTitle != "undated" {
		t.Errorf("after telling, matches = %+v, want only undated", matches)
	}
	if err := hub.MarkFreshMatchTold(ctx, uuid.New(), now); !errors.Is(err, store.ErrJobNotFound) {
		t.Errorf("marking an unknown job: %v, want ErrJobNotFound", err)
	}
}
