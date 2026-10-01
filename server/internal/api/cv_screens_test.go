package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

func TestAJobsCVScreenIsServedOnceWritten(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	routes := http.NewServeMux()
	api.RegisterCVRoutes(routes, hub, nil, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/1"})
	cv, _ := hub.SaveTailoredCV(ctx, owner, job.ID, resume.Resume{Basics: resume.Basics{Name: "Ada"}}, map[string]string{})

	if status, _ := send(t, http.MethodGet, server.URL+"/v1/jobs/"+job.ID.String()+"/cv/screen", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("before a screen: %d, want 404", status)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindRecruiterScreen)
	if err := hub.SaveCVScreen(ctx, store.CVScreen{JobID: job.ID, CVID: cv.ID, CVUpdatedAt: cv.UpdatedAt, PromptID: prompt.ID, Model: "local",
		Screen: json.RawMessage(`{"verdict":"likely_pass","reasons":[],"summary":"Fine."}`)}); err != nil {
		t.Fatal(err)
	}
	status, body := send(t, http.MethodGet, server.URL+"/v1/jobs/"+job.ID.String()+"/cv/screen", ownerToken, "")
	var served struct {
		Screen struct {
			Verdict string `json:"verdict"`
		} `json:"screen"`
	}
	if json.Unmarshal(body, &served); status != http.StatusOK || served.Screen.Verdict != "likely_pass" {
		t.Errorf("screen: %d %s", status, body)
	}
}
