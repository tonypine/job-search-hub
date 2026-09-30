package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/hubevents"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

var ownerToken = strings.Repeat("o", 64)

type apiUnderTest struct {
	pool     *pgxpool.Pool
	hub      *store.Store
	verifier auth.TokenVerifier
	url      string
}

func startAPI(t *testing.T) apiUnderTest {
	t.Helper()
	pool := testdatabase.New(t)
	hub := store.New(pool)
	verifier := tokens.NewVerifier(ownerToken, hub)
	requireOwner := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	routes := http.NewServeMux()
	api.RegisterAgentRunRoutes(routes, hub, requireOwner)
	api.RegisterCompanyRoutes(routes, hub, requireOwner)
	api.RegisterProfileRoutes(routes, hub, requireOwner)
	api.RegisterJobRoutes(routes, hub, stubPostings{}, stubRates{}, requireOwner)
	api.RegisterPipelineRoutes(routes, hub, requireOwner)
	api.RegisterJobCriteriaRoutes(routes, hub, requireOwner)
	api.RegisterConnectionRoutes(routes, hub, requireOwner)
	api.RegisterRecruiterRoutes(routes, hub, stubRates{}, requireOwner)
	api.RegisterCompanySuggestionRoutes(routes, hub, stubRates{}, requireOwner)
	api.RegisterLinkedInProfileRoutes(routes, hub, stubRates{}, requireOwner)
	broadcaster := hubevents.NewBroadcaster()
	api.RegisterUpdateRoutes(routes, hub, hubevents.NewRecorder(hub, broadcaster), requireOwner)
	api.RegisterTaskRoutes(routes, hub, hubevents.NewRecorder(hub, broadcaster), requireOwner)
	api.RegisterEventRoutes(routes, hub, broadcaster, requireOwner)
	api.RegisterClaudeSessionRoutes(routes, hub, stubRates{}, requireOwner)
	api.RegisterGoogleRoutes(routes, hub, nil, requireOwner)
	api.RegisterMailRoutes(routes, hub, nil, requireOwner)
	api.RegisterArtifactRoutes(routes, hub, requireOwner)
	api.RegisterAgentPromptRoutes(routes, hub, requireOwner)
	api.RegisterApplicationAnswerRoutes(routes, hub, requireOwner)
	api.RegisterProfileEntryRoutes(routes, hub, requireOwner)
	api.RegisterTaskRunRoutes(routes, hub, requireOwner)
	api.RegisterModelRoutingRoutes(routes, hub, requireOwner)
	api.RegisterWarmPathRoutes(routes, hub, requireOwner)
	api.RegisterDeviceRoutes(routes, hub, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return apiUnderTest{pool: pool, hub: hub, verifier: verifier, url: server.URL}
}

func send(t *testing.T, method, url, token, body string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

type startedRun struct {
	AgentRun store.AgentRun `json:"agent_run"`
	Token    string         `json:"token"`
}

func startTriage(t *testing.T, service apiUnderTest) startedRun {
	t.Helper()
	status, body := send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"company_triage","input":"stripe.com"}`)
	if status != http.StatusCreated {
		t.Fatalf("start: %d %s", status, body)
	}
	var started startedRun
	if err := json.Unmarshal(body, &started); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return started
}

func TestStartingARunReturnsItsTokenAndStoresOnlyTheHash(t *testing.T) {
	service := startAPI(t)
	started := startTriage(t, service)

	if started.Token == "" || started.AgentRun.Status != store.AgentRunRunning || started.AgentRun.Input != "stripe.com" {
		t.Fatalf("started = %+v", started)
	}
	var storedHash []byte
	if err := service.pool.QueryRow(context.Background(), `SELECT token_hash FROM agent_runs WHERE id = $1`, started.AgentRun.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if !bytes.Equal(storedHash, tokens.HashToken(started.Token)) || bytes.Contains(storedHash, []byte(started.Token)) {
		t.Fatal("the database does not hold exactly the token's hash")
	}
}

func TestFinishingARunRecordsItAndRevokesItsToken(t *testing.T) {
	service := startAPI(t)
	started := startTriage(t, service)
	finishURL := service.url + "/v1/agent-runs/" + started.AgentRun.ID.String() + "/finish"

	status, body := send(t, http.MethodPost, finishURL, ownerToken,
		`{"status":"succeeded","claude_session_id":"session-1","cost_usd_estimate":0.4217,"result":{"people_added":2}}`)
	if status != http.StatusOK {
		t.Fatalf("finish: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/agent-runs/"+started.AgentRun.ID.String(), ownerToken, "")
	var read struct {
		AgentRun store.AgentRun `json:"agent_run"`
	}
	if err := json.Unmarshal(body, &read); status != http.StatusOK || err != nil {
		t.Fatalf("get: %d %s", status, body)
	}
	run := read.AgentRun
	if run.Status != store.AgentRunSucceeded || run.ClaudeSessionID != "session-1" || run.CostUSDEstimate == nil || *run.CostUSDEstimate != "0.4217" {
		t.Fatalf("run = %+v", run)
	}

	if _, err := service.verifier(context.Background(), started.Token, nil); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("the finished run's token still verifies: err = %v", err)
	}
	if status, _ := send(t, http.MethodPost, finishURL, ownerToken, `{"status":"failed"}`); status != http.StatusConflict {
		t.Fatalf("second finish: %d, want 409", status)
	}
}

func TestAgentRunRoutesAreForTheOwnerOnly(t *testing.T) {
	service := startAPI(t)
	started := startTriage(t, service)
	runURL := service.url + "/v1/agent-runs/" + started.AgentRun.ID.String()

	for _, attempt := range []struct{ method, url, body string }{
		{http.MethodPost, service.url + "/v1/agent-runs", `{"kind":"company_triage","input":"clio.com"}`},
		{http.MethodGet, runURL, ""},
		{http.MethodPost, runURL + "/finish", `{"status":"succeeded"}`},
	} {
		if status, _ := send(t, attempt.method, attempt.url, started.Token, attempt.body); status != http.StatusForbidden {
			t.Errorf("agent token on %s %s: %d, want 403", attempt.method, attempt.url, status)
		}
		if status, _ := send(t, attempt.method, attempt.url, "", attempt.body); status != http.StatusUnauthorized {
			t.Errorf("no token on %s %s: %d, want 401", attempt.method, attempt.url, status)
		}
	}
}

func TestUnknownRunsAndBadRequests(t *testing.T) {
	service := startAPI(t)

	if status, _ := send(t, http.MethodGet, service.url+"/v1/agent-runs/7c9e6679-7425-40de-944b-e07fc1f90ae7", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("unknown run: %d, want 404", status)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"something_else","input":"x"}`); status != http.StatusBadRequest {
		t.Errorf("unknown kind: %d, want 400", status)
	}
}

func TestStartingARunReturnsTheRenderedPromptAndItsSchema(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := service.hub.SaveOwnerProfile(ctx, owner, "# Candidate\nSenior engineer."); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	status, body := send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"company_triage","input":"acme.com"}`)
	if status != http.StatusCreated {
		t.Fatalf("start: %d %s", status, body)
	}
	var started struct {
		AgentRun     store.AgentRun  `json:"agent_run"`
		Prompt       string          `json:"prompt"`
		ResultSchema json.RawMessage `json:"result_schema"`
	}
	if err := json.Unmarshal(body, &started); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(started.Prompt, "The company to research: acme.com") || !strings.Contains(started.Prompt, "Senior engineer.") {
		t.Fatalf("prompt lacks the company or the profile:\n%s", started.Prompt)
	}
	if started.AgentRun.AgentPromptVersion == nil || *started.AgentRun.AgentPromptVersion != 1 {
		t.Fatalf("agent_prompt_version = %v, want 1", started.AgentRun.AgentPromptVersion)
	}
	var schema map[string]any
	if err := json.Unmarshal(started.ResultSchema, &schema); err != nil || schema["type"] != "object" {
		t.Fatalf("result_schema = %s", started.ResultSchema)
	}
}

func TestAgentRunsAreListedLatestFirst(t *testing.T) {
	service := startAPI(t)
	for _, company := range []string{"first.example", "second.example"} {
		if status, answer := send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"company_triage","input":"`+company+`"}`); status != http.StatusCreated {
			t.Fatalf("start: %d %s", status, answer)
		}
	}
	status, answer := send(t, http.MethodGet, service.url+"/v1/agent-runs?limit=10", ownerToken, "")
	var listed struct {
		Runs []store.AgentRun `json:"runs"`
	}
	if json.Unmarshal(answer, &listed); status != http.StatusOK || len(listed.Runs) != 2 || listed.Runs[0].Input != "second.example" {
		t.Fatalf("agent runs: %d %s", status, answer)
	}
}
