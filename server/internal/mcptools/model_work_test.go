package mcptools_test

import (
	"context"
	"net/http/httptest"
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
	owner := mcptools.NewServer(hub, stubJobBoards{}, recordingSyncer{synced: make(chan store.JobBoard, 1)})
	mcptools.AddModelWorkTools(owner, &modelwork.Controls{Hub: hub, Queue: modelqueue.New(true, modelqueue.Settings{}), Runtime: idleRuntime{}, Facts: factsReader{read: read}})
	server := httptest.NewServer(mcptools.NewHandler(owner, mcptools.NewAgentServer(hub, stubJobBoards{}, recordingSyncer{}), tokens.NewVerifier(ownerToken, hub)))
	t.Cleanup(server.Close)
	session := connect(t, hubUnderTest{pool: pool, store: hub, url: server.URL}, ownerToken)

	if status := callTool[modelwork.Status](t, session, "get_model_work", map[string]any{}); !status.Paused || status.Runtime.State != modelruntime.StateStopped {
		t.Fatalf("status = %+v", status)
	}
	if status := callTool[modelwork.Status](t, session, "resume_model_work", map[string]any{}); status.Paused {
		t.Fatalf("after resuming: %+v", status)
	}
	if paused, _ := hub.GetModelWorkPaused(context.Background()); paused {
		t.Fatal("the resume wasn't saved")
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
