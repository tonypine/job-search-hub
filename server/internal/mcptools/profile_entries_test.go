package mcptools_test

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type profileEntriesResult struct {
	Entries []store.ProfileEntry `json:"entries"`
}

func TestAnAgentsEntriesWaitForTheOwnersConfirmation(t *testing.T) {
	hub := startHub(t)
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)
	owner := connect(t, hub, ownerToken)

	role := callTool[store.ProfileEntry](t, agent, "save_profile_entry", map[string]any{
		"kind": "role", "title": "Front-End Engineer", "organization": "Acme", "start_month": "2021-06", "source": "linkedin",
		"source_detail": "Positions.csv",
	})
	if role.ConfirmedAt != nil {
		t.Fatalf("an agent's new entry is confirmed: %+v", role)
	}
	result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "confirm_profile_entries", Arguments: map[string]any{"ids": []string{role.ID.String()}}})
	if err == nil && !result.IsError {
		t.Fatal("an agent confirmed an entry")
	}

	confirmed := callTool[profileEntriesResult](t, owner, "confirm_profile_entries", map[string]any{"ids": []string{role.ID.String()}})
	if len(confirmed.Entries) != 1 || confirmed.Entries[0].ConfirmedAt == nil {
		t.Fatalf("the owner's confirmation = %+v", confirmed)
	}
	listed := callTool[profileEntriesResult](t, agent, "list_profile_entries", map[string]any{"confirmed_only": true})
	if len(listed.Entries) != 1 {
		t.Fatalf("confirmed entries = %d, want 1", len(listed.Entries))
	}

	edited := callTool[store.ProfileEntry](t, agent, "save_profile_entry", map[string]any{
		"id": role.ID.String(), "kind": "role", "title": "Senior Front-End Engineer", "organization": "Acme", "start_month": "2021-06", "source": "linkedin",
	})
	if edited.ConfirmedAt != nil {
		t.Errorf("an agent's edit kept the confirmation: %+v", edited)
	}
}

func TestAnAgentsSessionListsNoProfileOwnerTool(t *testing.T) {
	hub := startHub(t)
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	listed, err := connect(t, hub, token).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	if !names["list_profile_entries"] || !names["save_profile_entry"] || names["confirm_profile_entries"] || names["delete_profile_entry"] {
		t.Errorf("an agent's profile tools = list %t, save %t, confirm %t, delete %t; want list and save only",
			names["list_profile_entries"], names["save_profile_entry"], names["confirm_profile_entries"], names["delete_profile_entry"])
	}
}
