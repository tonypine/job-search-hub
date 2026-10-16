package mcptools_test

import (
	"context"
	"maps"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/mcptools"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/modelruntime"
	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type idleRuntime struct{}

func (idleRuntime) Status() modelruntime.Status {
	return modelruntime.Status{State: modelruntime.StateStopped}
}

type factsReader struct{ read chan uuid.UUID }

func (reader factsReader) ReadJobNow(_ context.Context, jobID uuid.UUID) error {
	reader.read <- jobID
	return nil
}

func TestTheOwnerSeesPausesAndDispatchesModelWork(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	read := make(chan uuid.UUID, 1)
	owner := mcptools.NewServer(hub, stubJobBoards{}, recordingSyncer{synced: make(chan store.JobBoard, 1)}, noRates{})
	pauses, err := hub.GetRunKindPauses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mcptools.AddModelWorkTools(owner, &modelwork.Controls{Hub: hub, Queue: modelqueue.New(pauses, modelqueue.Settings{}), Runtime: idleRuntime{}, Facts: factsReader{read: read}})
	routeKind(t, hub, store.AgentPromptKindJobFacts)
	server := httptest.NewServer(mcptools.NewHandler(owner, mcptools.NewAgentServer(hub, stubJobBoards{}, recordingSyncer{}, noRates{}), tokens.NewVerifier(ownerToken, hub)))
	t.Cleanup(server.Close)
	session := connect(t, hubUnderTest{pool: pool, store: hub, url: server.URL}, ownerToken)

	if status := callTool[modelwork.Status](t, session, "get_model_work", map[string]any{}); !status.Paused || status.Runtime.State != modelruntime.StateStopped {
		t.Fatalf("status = %+v", status)
	}
	if status := callTool[modelwork.Status](t, session, "resume_model_work", map[string]any{}); status.Paused {
		t.Fatalf("after resuming: %+v", status)
	}
	if pauses, _ := hub.GetRunKindPauses(context.Background()); slices.Contains(slices.Collect(maps.Values(pauses)), true) {
		t.Fatalf("the resume wasn't saved for every kind: %v", pauses)
	}

	company, _, _ := hub.CreateCompany(context.Background(), store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	board, _ := hub.SetJobBoard(context.Background(), store.Actor{Kind: store.ActorOwner}, store.JobBoardInput{CompanyID: company.ID, Provider: "lever", BoardToken: "acme", Verified: true})
	hub.SyncBoardJobs(context.Background(), store.Actor{Kind: store.ActorSystem}, board, []store.JobPosting{{ExternalID: "1", Title: "Engineer", URL: "https://example.com/1", Description: "Build."}}, time.Now())
	jobs, _, _ := hub.ListJobs(context.Background(), store.JobFilter{})
	if queued := callTool[struct{ Queued bool }](t, session, "read_job_facts_now", map[string]any{"job_id": jobs[0].Job.ID}); !queued.Queued {
		t.Fatal("not queued")
	}
	select {
	case got := <-read:
		if got != jobs[0].Job.ID {
			t.Fatalf("read %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the read never started")
	}
}

// routeKind routes kind to a model on a provider of its own.
func routeKind(t *testing.T, hub *store.Store, kind string) {
	t.Helper()
	ctx := context.Background()
	provider, err := hub.SaveModelProvider(ctx, store.Actor{Kind: store.ActorOwner}, nil, store.ModelProviderInput{Name: "This Mac", BaseURL: "http://127.0.0.1:1/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTaskRoute(ctx, store.Actor{Kind: store.ActorOwner}, kind, store.TaskRouteInput{ProviderID: provider.ID, Model: "qwen3.8-27b"}); err != nil {
		t.Fatal(err)
	}
}

func TestAClaudeSessionListsAJobsRunsAndStartsOne(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	owner := mcptools.NewServer(hub, stubJobBoards{}, recordingSyncer{synced: make(chan store.JobBoard, 1)}, noRates{})
	mcptools.AddModelWorkTools(owner, &modelwork.Controls{Hub: hub, Queue: modelqueue.New(nil, modelqueue.Settings{}), Runtime: idleRuntime{},
		Facts: factsReader{read: make(chan uuid.UUID, 1)}})
	server := httptest.NewServer(mcptools.NewHandler(owner, mcptools.NewAgentServer(hub, stubJobBoards{}, recordingSyncer{}, noRates{}), tokens.NewVerifier(ownerToken, hub)))
	t.Cleanup(server.Close)
	session := connect(t, hubUnderTest{pool: pool, store: hub, url: server.URL}, ownerToken)
	routeKind(t, hub, store.AgentPromptKindJobFacts)
	job, _, err := hub.AddManualJob(ctx, store.Actor{Kind: store.ActorOwner}, store.ManualJobInput{Title: "Engineer", URL: "https://acme.example/1", Description: "Build."})
	if err != nil {
		t.Fatal(err)
	}

	listed := callTool[struct{ Kinds []modelwork.RunnableKind }](t, session, "list_runnable_kinds", map[string]any{"job_id": job.ID})
	if len(listed.Kinds) != 7 || listed.Kinds[0].ID != store.AgentPromptKindJobFacts || !listed.Kinds[0].CanRun || listed.Kinds[0].Route == nil {
		t.Fatalf("a job's kinds = %+v", listed.Kinds)
	}
	if reason := callFailingTool(t, session, "start_run", map[string]any{"kind": "recruiter_screen", "job_id": job.ID}); !strings.Contains(reason, "no model is routed") {
		t.Errorf("an unrouted screen: %s", reason)
	}
	if reason := callFailingTool(t, session, "list_runnable_kinds", map[string]any{}); !strings.Contains(reason, "job_id or company_id") {
		t.Errorf("no subject: %s", reason)
	}

	started := callTool[modelwork.StartedRun](t, session, "start_run", map[string]any{"kind": "job_fix", "job_id": job.ID, "note": "The title is Staff Engineer."})
	queued, _ := hub.ListTasks(ctx, store.TaskQueued, 10)
	if started.TaskID == nil || len(queued) != 1 || queued[0].ID != *started.TaskID || queued[0].Kind != store.TaskFixJob || queued[0].RunID == nil ||
		*queued[0].RunID != started.ID || queued[0].Input != "The title is Staff Engineer." {
		t.Fatalf("started %+v; the Mac's tasks = %+v", started, queued)
	}
}
