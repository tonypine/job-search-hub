package mcptools_test

import (
	"context"
	"slices"
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

type foundJob struct {
	ID            string
	Title         string
	Company       string
	Location      string
	URL           string
	FitLevel      string `json:"fit_level"`
	PipelinePhase string `json:"pipeline_phase"`
	Closed        bool
}

type foundJobs struct {
	Jobs  []foundJob
	Total int
	Pages int
}

func (found foundJobs) getSortedTitles() []string {
	titles := make([]string, len(found.Jobs))
	for index, job := range found.Jobs {
		titles[index] = job.Title
	}
	slices.Sort(titles)
	return titles
}

func TestJobsAreFoundByCompanyTitleAndPhaseByOwnerAndAgent(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	system := store.Actor{Kind: store.ActorSystem}
	acme, _, _ := hub.store.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	other, _, _ := hub.store.CreateCompany(ctx, owner, store.NewCompany{Name: "Globex", Domain: "globex.example"})
	acmeBoard, _ := hub.store.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: acme.ID, Provider: "greenhouse", BoardToken: "acme", Verified: true})
	otherBoard, _ := hub.store.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: other.ID, Provider: "lever", BoardToken: "acme", Verified: true})
	acmePostings := []store.JobPosting{
		{ExternalID: "1", Title: "Senior Software Engineer, Full Stack", Location: "Americas Remote", URL: "https://acme.example/jobs/1"},
		{ExternalID: "2", Title: "Staff Software Engineer, Product", Location: "Worldwide", URL: "https://acme.example/jobs/2"},
		{ExternalID: "3", Title: "Senior Designer", Location: "Remote", URL: "https://acme.example/jobs/3"},
	}
	if _, err := hub.store.SyncBoardJobs(ctx, system, acmeBoard, acmePostings, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.store.SyncBoardJobs(ctx, system, acmeBoard, acmePostings[:2], time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.store.SyncBoardJobs(ctx, system, otherBoard, []store.JobPosting{{ExternalID: "9", Title: "Senior Engineer", URL: "https://globex.example/9"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	saved, _, _ := hub.store.ListJobs(ctx, store.JobFilter{Title: "full stack"})
	if _, _, err := hub.store.AddApplication(ctx, owner, store.ApplicationInput{JobID: &saved[0].Job.ID}); err != nil {
		t.Fatal(err)
	}

	_, agentToken := startAgentRun(t, hub, time.Now().Add(time.Hour))
	for name, token := range map[string]string{"owner": ownerToken, "agent": agentToken} {
		session := connect(t, hub, token)

		byCompany := callTool[foundJobs](t, session, "find_jobs", map[string]any{"domain": "https://acme.example/careers"})
		if titles := byCompany.getSortedTitles(); !slices.Equal(titles, []string{"Senior Software Engineer, Full Stack", "Staff Software Engineer, Product"}) {
			t.Errorf("%s: Acme's open jobs = %v", name, titles)
		}
		for _, job := range byCompany.Jobs {
			if job.Company != "Acme" || job.Location == "" || job.URL == "" || job.FitLevel == "" {
				t.Errorf("%s: a found job lacks its details: %+v", name, job)
			}
			if (job.PipelinePhase == "Saved") != (job.Title == "Senior Software Engineer, Full Stack") {
				t.Errorf("%s: %s has phase %q", name, job.Title, job.PipelinePhase)
			}
		}
		if byTitle := callTool[foundJobs](t, session, "find_jobs", map[string]any{"company_id": acme.ID, "title": "senior"}); !slices.Equal(byTitle.getSortedTitles(), []string{"Senior Software Engineer, Full Stack"}) {
			t.Errorf("%s: Acme's open senior jobs = %v", name, byTitle.getSortedTitles())
		}
		if inSaved := callTool[foundJobs](t, session, "find_jobs", map[string]any{"pipeline_phase": "saved"}); !slices.Equal(inSaved.getSortedTitles(), []string{"Senior Software Engineer, Full Stack"}) {
			t.Errorf("%s: Saved = %v", name, inSaved.getSortedTitles())
		}
		if offPipeline := callTool[foundJobs](t, session, "find_jobs", map[string]any{"pipeline_phase": "none"}); !slices.Equal(offPipeline.getSortedTitles(), []string{"Senior Engineer", "Staff Software Engineer, Product"}) {
			t.Errorf("%s: not on the pipeline = %v", name, offPipeline.getSortedTitles())
		}
		withClosed := callTool[foundJobs](t, session, "find_jobs", map[string]any{"domain": "acme.example", "include_closed": true})
		isClosedDesigner := func(job foundJob) bool { return job.Title == "Senior Designer" && job.Closed }
		if withClosed.Total != 3 || !slices.ContainsFunc(withClosed.Jobs, isClosedDesigner) {
			t.Errorf("%s: with closed jobs = %+v", name, withClosed)
		}
		if text := callRefusedTool(t, session, "find_jobs", map[string]any{"pipeline_phase": "Shortlist"}); !strings.Contains(text, "Saved") {
			t.Errorf("%s: an unknown phase: %s", name, text)
		}
		if text := callRefusedTool(t, session, "find_jobs", map[string]any{"domain": "initech.example"}); !strings.Contains(text, "no company at initech.example") {
			t.Errorf("%s: an unknown company: %s", name, text)
		}
	}
}

func TestAnAgentCorrectsAJobWithReasonsAndUnknownFieldsAreRefused(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	expiresAt := time.Now().Add(48 * time.Hour)
	if _, err := hub.store.SyncFeedJobs(ctx, store.Actor{Kind: store.ActorSystem}, store.JobSourceHimalayas, []store.JobPosting{{
		ExternalID: "7", CompanyName: "Join our winning team!", Title: "Frontend Engineer - Track&Field - São Paulo", URL: "https://indeed.example/7", ExpiresAt: &expiresAt,
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.store.ListJobs(ctx, store.JobFilter{})
	run, agentToken := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, agentToken)

	fixed := callTool[store.Job](t, agent, "update_job", map[string]any{
		"job_id": jobs[0].Job.ID, "title": "Frontend Engineer", "company_name": "Track&Field",
		"reasons": map[string]any{"title": "the title carries the company and city", "company_name": "the company field holds a slogan"},
	})
	if fixed.Title != "Frontend Engineer" {
		t.Fatalf("fixed = %+v", fixed)
	}
	var byRun int
	if err := hub.pool.QueryRow(ctx, `SELECT count(*) FROM changes WHERE entity_id = $1 AND operation = 'fix' AND agent_run_id = $2`, jobs[0].Job.ID, run.ID).Scan(&byRun); err != nil || byRun != 2 {
		t.Errorf("fix changes by the run = %d, %v", byRun, err)
	}
	if text := callRefusedTool(t, agent, "update_job", map[string]any{
		"job_id": jobs[0].Job.ID, "salary": "100k", "reasons": map[string]any{"salary": "x"},
	}); !strings.Contains(text, "salary") {
		t.Errorf("an unknown field: %s", text)
	}
}
