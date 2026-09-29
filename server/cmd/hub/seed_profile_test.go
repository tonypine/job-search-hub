package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/agents/profileseed"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

func seedResultLine() string {
	encoded, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "session_id": "session-1", "total_cost_usd": 0.21,
		"structured_output": map[string]any{
			"entries_added": 5, "entries_updated": 0,
			"conflicts": []string{"The CV says Acme is current; LinkedIn says it ended in 2025-09."},
			"summary":   "Two roles with three cases.",
		},
	})
	return string(encoded)
}

func TestSeedingTheProfileGivesTheAgentTheLinkedInProfileAndRecordsAnUpdate(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if err := hub.store.SaveLinkedInProfile(ctx, owner, store.LinkedInProfile{
		Headline:  "Front-End Engineer",
		Positions: []store.LinkedInPosition{{Company: "Acme", Title: "Front-End Engineer", Description: "Built the checkout", StartedOn: "Jun 2021"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.store.ImportLinkedInEndorsements(ctx, owner, []store.NewLinkedInEndorsement{
		{Direction: "received", Skill: "React", FirstName: "Sam", LastName: "Lee", ProfileURL: "https://www.linkedin.com/in/sam-lee-example", Status: "accepted"},
		{Direction: "received", Skill: "React", FirstName: "Ana", LastName: "Ruiz", ProfileURL: "https://www.linkedin.com/in/ana-ruiz-example", Status: "accepted"},
		{Direction: "received", Skill: "React", FirstName: "Bo", LastName: "Kim", ProfileURL: "https://www.linkedin.com/in/bo-kim-example", Status: "rejected"},
	}); err != nil {
		t.Fatal(err)
	}
	argumentsPath := installFakeClaude(t, initLine(""), seedResultLine())

	var out bytes.Buffer
	if err := seedProfile(ctx, hub.config, triageOptions{}, &out); err != nil {
		t.Fatalf("seed: %v\n%s", err, out.String())
	}
	status, _, _, _ := readAgentRun(t, hub.pool)
	if status != store.AgentRunSucceeded {
		t.Fatalf("run status = %s", status)
	}
	recorded, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Built the checkout", `"skill": "React"`, `"count": 2`} {
		if !strings.Contains(string(recorded), want) {
			t.Errorf("the agent's prompt lacks %q", want)
		}
	}
	if !strings.Contains(out.String(), "to settle: The CV says Acme is current") {
		t.Errorf("output lacks the conflict:\n%s", out.String())
	}
	updates, err := hub.store.ListUpdates(ctx, 10, false)
	if err != nil || len(updates.Updates) != 1 || updates.Updates[0].Title != "Built 5 entries from your CV and LinkedIn" ||
		!strings.Contains(updates.Updates[0].Body, "To settle: The CV says Acme is current") {
		t.Fatalf("updates = %+v, %v", updates.Updates, err)
	}
}

func TestASeedThatAddsNothingSaysSo(t *testing.T) {
	title, body := describeSeedUpdate(profileseed.Result{Summary: "Nothing new.", EntriesUpdated: 2})
	if title != "Your knowledge base already had what your CV and LinkedIn say" || !strings.Contains(body, "Updated 2 existing entries.") {
		t.Errorf("title %q, body %q", title, body)
	}
}
