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

func TestTheOwnerDismissesAndRestoresACardAndAnAgentCant(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, _ := hub.store.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	card, _, err := hub.store.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &company.ID})
	if err != nil {
		t.Fatal(err)
	}
	session := connect(t, hub, ownerToken)

	callTool[store.Application](t, session, "dismiss_application", map[string]any{"application_id": card.ID, "note": "agency"})
	if dismissed, _ := hub.store.ListDismissedPipelineCards(ctx); len(dismissed) != 1 || dismissed[0].DismissalReason != "not a good fit: agency" {
		t.Fatalf("dismissed cards = %+v", dismissed)
	}
	callTool[store.Application](t, session, "restore_application", map[string]any{"application_id": card.ID})
	if onBoard, _ := hub.store.ListPipelineCards(ctx); len(onBoard) != 1 {
		t.Fatalf("board after restoring = %+v", onBoard)
	}

	_, agentToken := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, agentToken)
	for _, tool := range []string{"dismiss_application", "restore_application"} {
		if text := callRefusedTool(t, agent, tool, map[string]any{"application_id": card.ID}); !strings.Contains(text, "unknown tool") {
			t.Errorf("%s from an agent: %s", tool, text)
		}
	}
}

func TestTheOwnerDecidesOnAJobAndAnAgentCant(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	job, _, _ := hub.store.AddManualJob(ctx, store.Actor{Kind: store.ActorOwner}, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/1"})
	session := connect(t, hub, ownerToken)

	if decision := callTool[store.JobDecision](t, session, "decide_job", map[string]any{"job_id": job.ID, "decision": "later"}); decision.Decision != "later" {
		t.Fatalf("decision = %+v", decision)
	}
	_, agentToken := startAgentRun(t, hub, time.Now().Add(time.Hour))
	if text := callRefusedTool(t, connect(t, hub, agentToken), "decide_job", map[string]any{"job_id": job.ID, "decision": "skip"}); !strings.Contains(text, "unknown tool") {
		t.Errorf("decide_job from an agent: %s", text)
	}
}
