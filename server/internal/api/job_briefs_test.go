package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type recordingBriefWriter struct{ written chan uuid.UUID }

func (writer recordingBriefWriter) WriteFullBrief(_ context.Context, jobID uuid.UUID) error {
	writer.written <- jobID
	return nil
}

func TestAFullBriefIsQueuedForAKnownJob(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	writer := recordingBriefWriter{written: make(chan uuid.UUID, 1)}
	routes := http.NewServeMux()
	api.RegisterJobBriefRoutes(routes, hub, writer, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	job, _, _ := hub.AddManualJob(context.Background(), store.Actor{Kind: store.ActorOwner}, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/1"})

	if status, body := send(t, http.MethodPost, server.URL+"/v1/jobs/"+job.ID.String()+"/brief/full", ownerToken, ""); status != http.StatusAccepted {
		t.Fatalf("known job: %d %s", status, body)
	}
	select {
	case written := <-writer.written:
		if written != job.ID {
			t.Errorf("briefed %s, want %s", written, job.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the full brief was never written")
	}
	if status, _ := send(t, http.MethodPost, server.URL+"/v1/jobs/"+uuid.NewString()+"/brief/full", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("unknown job: %d, want 404", status)
	}

	off := http.NewServeMux()
	api.RegisterJobBriefRoutes(off, hub, nil, requireOwner)
	offServer := httptest.NewServer(off)
	t.Cleanup(offServer.Close)
	if status, _ := send(t, http.MethodPost, offServer.URL+"/v1/jobs/"+job.ID.String()+"/brief/full", ownerToken, ""); status != http.StatusServiceUnavailable {
		t.Errorf("without Claude: %d, want 503", status)
	}
}
