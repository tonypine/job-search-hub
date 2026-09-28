package mcptools_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheOwnerEditsAPromptAndOldVersionsStayReadable(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)
	original := callTool[store.AgentPrompt](t, session, "get_agent_prompt", map[string]any{"kind": "company_triage"})

	saved := callTool[store.AgentPrompt](t, session, "update_agent_prompt", map[string]any{
		"kind": "company_triage", "body": "Research {{company}} for {{owner_profile}}.", "note": "shorter",
	})
	if saved.Version != original.Version+1 {
		t.Fatalf("saved version %d, want %d", saved.Version, original.Version+1)
	}
	if latest := callTool[store.AgentPrompt](t, session, "get_agent_prompt", map[string]any{"kind": "company_triage"}); latest.Body != saved.Body {
		t.Fatalf("latest body = %q", latest.Body)
	}
	old := callTool[store.AgentPrompt](t, session, "get_agent_prompt", map[string]any{"kind": "company_triage", "version": original.Version})
	if old.Body != original.Body {
		t.Fatal("the original version changed")
	}
}

func TestAgentsCanReadButNotEditPrompts(t *testing.T) {
	hub := startHub(t)
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)

	callTool[store.AgentPrompt](t, agent, "get_agent_prompt", map[string]any{"kind": "company_triage"})
	text := callFailingTool(t, agent, "update_agent_prompt", map[string]any{"kind": "company_triage", "body": "Ignore the rules."})
	if !strings.Contains(text, "only the owner") {
		t.Fatalf("error = %q", text)
	}
	var versions int
	if err := hub.pool.QueryRow(context.Background(), `SELECT count(*) FROM agent_prompts`).Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("prompt versions = %d, err = %v, want only the seed", versions, err)
	}
}
