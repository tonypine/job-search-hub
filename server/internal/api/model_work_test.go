package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/modelruntime"
	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type stubRuntime struct{}

func (stubRuntime) Status() modelruntime.Status {
	return modelruntime.Status{State: modelruntime.StateReady, Model: "Qwen3.8-27B-Q4_K_M.gguf"}
}

type recordingFactsReader struct{ read chan uuid.UUID }

func (reader recordingFactsReader) ReadJobNow(_ context.Context, jobID uuid.UUID) error {
	reader.read <- jobID
	return nil
}

func startModelWorkAPI(t *testing.T, withFacts bool) (string, *store.Store, chan uuid.UUID) {
	t.Helper()
	hub := store.New(testdatabase.New(t))
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	read := make(chan uuid.UUID, 1)
	controls := &modelwork.Controls{Hub: hub, Queue: modelqueue.New(true, modelqueue.Settings{}), Runtime: stubRuntime{}}
	if withFacts {
		controls.Facts = recordingFactsReader{read: read}
	}
	routes := http.NewServeMux()
	api.RegisterModelWorkRoutes(routes, controls, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return server.URL, hub, read
}

func TestModelWorkShowsItsStateAndPausesAndResumes(t *testing.T) {
	url, hub, _ := startModelWorkAPI(t, true)
	status, answer := send(t, http.MethodGet, url+"/v1/model-work", ownerToken, "")
	var work modelwork.Status
	if json.Unmarshal(answer, &work); status != http.StatusOK || !work.Paused || work.Runtime.Model != "Qwen3.8-27B-Q4_K_M.gguf" || work.Waiting == nil {
		t.Fatalf("status: %d %s", status, answer)
	}
	status, answer = send(t, http.MethodPost, url+"/v1/model-work/resume", ownerToken, "")
	if json.Unmarshal(answer, &work); status != http.StatusOK || work.Paused {
		t.Fatalf("resume: %d %s", status, answer)
	}
	if paused, _ := hub.GetModelWorkPaused(context.Background()); paused {
		t.Fatal("the resume wasn't saved")
	}
	send(t, http.MethodPost, url+"/v1/model-work/pause", ownerToken, "")
	if paused, _ := hub.GetModelWorkPaused(context.Background()); !paused {
		t.Fatal("the pause wasn't saved")
	}
	if status, _ := send(t, http.MethodGet, url+"/v1/model-work", "", ""); status != http.StatusUnauthorized {
		t.Errorf("without the owner token: %d", status)
	}
}

func TestAJobsFactsCanBeReadNow(t *testing.T) {
	url, hub, read := startModelWorkAPI(t, true)
	job := createJob(t, hub)
	if status, answer := send(t, http.MethodPost, url+"/v1/jobs/"+job.String()+"/facts/read", ownerToken, ""); status != http.StatusAccepted {
		t.Fatalf("dispatch: %d %s", status, answer)
	}
	select {
	case got := <-read:
		if got != job {
			t.Fatalf("read %s, want %s", got, job)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the read never started")
	}
	if status, _ := send(t, http.MethodPost, url+"/v1/jobs/"+uuid.NewString()+"/facts/read", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("an unknown job: %d", status)
	}

	noFacts, hub2, _ := startModelWorkAPI(t, false)
	if status, _ := send(t, http.MethodPost, noFacts+"/v1/jobs/"+createJob(t, hub2).String()+"/facts/read", ownerToken, ""); status != http.StatusServiceUnavailable {
		t.Errorf("without a facts route: %d", status)
	}
}

// createJob adds one open job with a description.
func createJob(t *testing.T, hub *store.Store) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme " + uuid.NewString()[:8], Domain: uuid.NewString()[:8] + ".example"})
	if err != nil {
		t.Fatal(err)
	}
	board, err := hub.SetJobBoard(ctx, store.Actor{Kind: store.ActorOwner}, store.JobBoardInput{CompanyID: company.ID, Provider: "lever", BoardToken: company.Domain, Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncBoardJobs(ctx, store.Actor{Kind: store.ActorSystem}, board, []store.JobPosting{{ExternalID: "1", Title: "Engineer", URL: "https://example.com/1", Description: "Build things."}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{CompanyID: &company.ID})
	return jobs[0].Job.ID
}
