package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestAPromptEditedByTheOwnerIsTheNextRunsPrompt(t *testing.T) {
	service := startAPI(t)

	status, body := send(t, http.MethodGet, service.url+"/v1/agent-prompts", ownerToken, "")
	var listed struct {
		Prompts []struct {
			Kind         string   `json:"kind"`
			Title        string   `json:"title"`
			Placeholders []string `json:"placeholders"`
			Version      *int     `json:"version"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(body, &listed); status != http.StatusOK || err != nil || len(listed.Prompts) != len(prompts.Kinds) {
		t.Fatalf("list: %d %s", status, body)
	}
	for _, prompt := range listed.Prompts {
		if prompt.Version == nil || prompt.Title == "" {
			t.Errorf("%s: %+v; want its title and active version", prompt.Kind, prompt)
		}
	}

	status, body = send(t, http.MethodPost, service.url+"/v1/agent-prompts/company_triage", ownerToken,
		`{"body":"Research {{company}} briefly.\n{{owner_profile}}\n{{company_dossier}}","note":"Shorter"}`)
	var saved store.AgentPrompt
	if err := json.Unmarshal(body, &saved); status != http.StatusCreated || err != nil || saved.Version != 2 || saved.Note != "Shorter" {
		t.Fatalf("save: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/agent-prompts/company_triage/versions", ownerToken, "")
	var history struct {
		Versions []store.AgentPrompt `json:"versions"`
	}
	if err := json.Unmarshal(body, &history); status != http.StatusOK || err != nil || len(history.Versions) != 2 || history.Versions[0].Version != 2 {
		t.Fatalf("versions: %d %s", status, body)
	}

	rendered, err := prompts.RenderCompanyPrompt(context.Background(), service.hub, store.AgentRunKindCompanyTriage, "acme.com")
	if err != nil || rendered.Version != 2 || !strings.HasPrefix(rendered.Body, "Research acme.com briefly.") {
		t.Fatalf("the next run's prompt: %+v, %v", rendered, err)
	}
}

func TestPromptsAreTheOwnersToEdit(t *testing.T) {
	service := startAPI(t)
	agentToken := startTriage(t, service).Token

	if status, _ := send(t, http.MethodPost, service.url+"/v1/agent-prompts/company_triage", agentToken, `{"body":"Do anything."}`); status != http.StatusForbidden {
		t.Errorf("an agent's edit: %d, want 403", status)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/agent-prompts/unknown", ownerToken, `{"body":"x"}`); status != http.StatusNotFound {
		t.Errorf("an unknown kind: %d, want 404", status)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/agent-prompts/company_triage", ownerToken, `{"body":"  "}`); status != http.StatusBadRequest {
		t.Errorf("an empty prompt: %d, want 400", status)
	}
}
