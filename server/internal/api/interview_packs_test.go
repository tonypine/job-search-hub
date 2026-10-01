package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

func TestAJobsInterviewPackIsServedOnceWritten(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	routes := http.NewServeMux()
	api.RegisterInterviewPackRoutes(routes, hub, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	ctx := context.Background()
	job, _, _ := hub.AddManualJob(ctx, store.Actor{Kind: store.ActorOwner}, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/1"})

	if status, _ := send(t, http.MethodGet, server.URL+"/v1/jobs/"+job.ID.String()+"/interview-pack", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("before a pack: %d, want 404", status)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindInterviewPrep)
	if err := hub.SaveInterviewPack(ctx, store.InterviewPack{JobID: job.ID, PromptID: prompt.ID, KnowledgeHash: "h", Model: "local",
		Pack: json.RawMessage(`{"questions":[{"question":"Why us?","reason":"","stories":[],"talking_points":""}],"role_gaps":[]}`)}); err != nil {
		t.Fatal(err)
	}
	status, body := send(t, http.MethodGet, server.URL+"/v1/jobs/"+job.ID.String()+"/interview-pack", ownerToken, "")
	var served struct {
		Pack struct {
			Questions []struct {
				Question string `json:"question"`
			} `json:"questions"`
		} `json:"pack"`
	}
	if json.Unmarshal(body, &served); status != http.StatusOK || len(served.Pack.Questions) != 1 || served.Pack.Questions[0].Question != "Why us?" {
		t.Errorf("pack: %d %s", status, body)
	}
}
