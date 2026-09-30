package mcptools_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheOwnerDismissesAndRestoresJobsAndAnAgentCant(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, _ := hub.store.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	board, _ := hub.store.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: company.ID, Provider: "lever", BoardToken: "acme", Verified: true})
	postings := []store.JobPosting{{ExternalID: "1", Title: "Agency Role", URL: "https://example.com/1"}, {ExternalID: "2", Title: "Engineer", URL: "https://example.com/2"}}
	if _, err := hub.store.SyncBoardJobs(ctx, store.Actor{Kind: store.ActorSystem}, board, postings, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.store.ListJobs(ctx, store.JobFilter{Query: "agency"})
	session := connect(t, hub, ownerToken)

	type jobsOutput struct{ Jobs []store.Job }
	dismissed := callTool[jobsOutput](t, session, "dismiss_job", map[string]any{"job_ids": []any{jobs[0].Job.ID}, "reason": " an agency "})
	if len(dismissed.Jobs) != 1 || dismissed.Jobs[0].DismissalReason != "an agency" {
		t.Fatalf("dismissed = %+v", dismissed)
	}
	if open, _, _ := hub.store.ListJobs(ctx, store.JobFilter{}); len(open) != 1 || open[0].Job.Title != "Engineer" {
		t.Fatalf("open after dismissing = %+v", open)
	}
	if restored := callTool[jobsOutput](t, session, "restore_job", map[string]any{"job_ids": []any{jobs[0].Job.ID}}); len(restored.Jobs) != 1 || restored.Jobs[0].DismissedAt != nil {
		t.Fatalf("restored = %+v", restored)
	}

	_, agentToken := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, agentToken)
	for _, tool := range []string{"dismiss_job", "restore_job"} {
		if text := callRefusedTool(t, agent, tool, map[string]any{"job_ids": []any{jobs[0].Job.ID}}); !strings.Contains(text, "unknown tool") {
			t.Errorf("%s from an agent: %s", tool, text)
		}
	}
}
