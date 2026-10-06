package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/drain"
)

type drainStatus struct {
	Draining bool `json:"draining"`
	Running  []struct {
		Type       string `json:"type"`
		Kind       string `json:"kind"`
		Subject    string `json:"subject"`
		AgeSeconds *int   `json:"age_seconds"`
	} `json:"running"`
}

func readDrain(t *testing.T, service apiUnderTest, method string) drainStatus {
	t.Helper()
	status, body := send(t, method, service.url+"/v1/drain", ownerToken, "")
	var read drainStatus
	if err := json.Unmarshal(body, &read); status != http.StatusOK || err != nil {
		t.Fatalf("%s /v1/drain: %d %s", method, status, body)
	}
	return read
}

func TestNoAgentRunStartsWhileDrainingAndTheRunningWorkIsListedUntilItFinishes(t *testing.T) {
	service := startAPI(t)
	started := startTriage(t, service)
	endClaudeRun, err := service.drainer.Begin(drain.TypeClaudeRun, "job_brief", "job-1")
	if err != nil {
		t.Fatal(err)
	}

	drained := readDrain(t, service, http.MethodPost)
	if !drained.Draining || len(drained.Running) != 2 {
		t.Fatalf("drain = %+v, want draining with the agent run and the claude run", drained)
	}
	agentRun, claudeRun := drained.Running[0], drained.Running[1]
	if agentRun.Type != drain.TypeAgentRun {
		// The database's clock and the server's may disagree on which started first.
		agentRun, claudeRun = claudeRun, agentRun
	}
	if agentRun.Type != drain.TypeAgentRun || agentRun.Kind != "company_triage" || agentRun.Subject != "stripe.com" || agentRun.AgeSeconds == nil {
		t.Errorf("agent run = %+v", agentRun)
	}
	if claudeRun.Type != drain.TypeClaudeRun || claudeRun.Kind != "job_brief" || claudeRun.Subject != "job-1" || claudeRun.AgeSeconds == nil {
		t.Errorf("claude run = %+v", claudeRun)
	}

	request, _ := http.NewRequest(http.MethodPost, service.url+"/v1/agent-runs", strings.NewReader(`{"kind":"company_triage","input":"clio.com"}`))
	request.Header.Set("Authorization", "Bearer "+ownerToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || response.Header.Get("Retry-After") == "" {
		t.Fatalf("start while draining: %d, Retry-After %q, want 503 with Retry-After", response.StatusCode, response.Header.Get("Retry-After"))
	}

	// The running work finishes, and reads and the owner's writes go on.
	finishURL := service.url + "/v1/agent-runs/" + started.AgentRun.ID.String() + "/finish"
	if status, body := send(t, http.MethodPost, finishURL, ownerToken, `{"status":"succeeded","result":{}}`); status != http.StatusOK {
		t.Fatalf("finish while draining: %d %s", status, body)
	}
	endClaudeRun()
	if status, body := send(t, http.MethodPut, service.url+"/v1/profile", ownerToken, `{"body":"# Candidate\nSenior engineer."}`); status != http.StatusOK {
		t.Fatalf("an owner write while draining: %d %s", status, body)
	}
	if status, body := send(t, http.MethodGet, service.url+"/v1/agent-runs", ownerToken, ""); status != http.StatusOK || !strings.Contains(string(body), "stripe.com") {
		t.Fatalf("a read while draining: %d %s", status, body)
	}
	if listed := readDrain(t, service, http.MethodGet); !listed.Draining || len(listed.Running) != 0 {
		t.Fatalf("drain once the work finished = %+v", listed)
	}

	if cancelled := readDrain(t, service, http.MethodDelete); cancelled.Draining {
		t.Fatalf("drain after DELETE = %+v", cancelled)
	}
	startTriage(t, service)
}

func TestDrainRoutesAreForTheOwnerOnly(t *testing.T) {
	service := startAPI(t)
	started := startTriage(t, service)
	for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		if status, _ := send(t, method, service.url+"/v1/drain", started.Token, ""); status != http.StatusForbidden {
			t.Errorf("agent token on %s: %d, want 403", method, status)
		}
		if status, _ := send(t, method, service.url+"/v1/drain", "", ""); status != http.StatusUnauthorized {
			t.Errorf("no token on %s: %d, want 401", method, status)
		}
	}
	if service.drainer.Draining() {
		t.Fatal("a refused request started a drain")
	}
}
