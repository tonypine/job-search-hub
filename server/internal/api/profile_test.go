package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheProfileRoundTripsAndRecordsTheSave(t *testing.T) {
	service := startAPI(t)

	status, body := send(t, http.MethodPut, service.url+"/v1/profile", ownerToken, `{"body":"# Candidate\nSenior engineer."}`)
	if status != http.StatusOK {
		t.Fatalf("put: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/profile", ownerToken, "")
	var profile store.OwnerProfile
	if err := json.Unmarshal(body, &profile); status != http.StatusOK || err != nil || profile.Body != "# Candidate\nSenior engineer." {
		t.Fatalf("get: %d %s", status, body)
	}

	var saves int
	err := service.pool.QueryRow(context.Background(), `SELECT count(*) FROM changes WHERE entity_type = 'owner_profile' AND actor_kind = 'owner'`).Scan(&saves)
	if err != nil || saves != 1 {
		t.Fatalf("profile changes = %d, err = %v, want 1", saves, err)
	}
}

func TestTheActiveVersionOfAPromptIsServed(t *testing.T) {
	service := startAPI(t)

	status, body := send(t, http.MethodGet, service.url+"/v1/agent-prompts/outreach_draft", ownerToken, "")
	var prompt store.AgentPrompt
	if err := json.Unmarshal(body, &prompt); status != http.StatusOK || err != nil || prompt.Version != 1 || !strings.Contains(prompt.Body, "Draft only") {
		t.Fatalf("outreach prompt: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/agent-prompts/unknown", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("unknown prompt: %d, want 404", status)
	}
}
