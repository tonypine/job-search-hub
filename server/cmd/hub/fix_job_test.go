package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func jobFixResultLine(jobID string, fixedFields []string, summary string) string {
	encoded, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "session_id": "session-1", "total_cost_usd": 0.05,
		"structured_output": map[string]any{"job_id": jobID, "fixed_fields": fixedFields, "summary": summary},
	})
	return string(encoded)
}

func TestAJobFixGivesTheAgentTheJobAndTheNote(t *testing.T) {
	hub := startHub(t)
	job, _, err := hub.store.AddManualJob(context.Background(), store.Actor{Kind: store.ActorOwner},
		store.ManualJobInput{Title: "Frontend Engineer - Track&Field - São Paulo", URL: "https://indeed.example/7"})
	if err != nil {
		t.Fatal(err)
	}
	note := "the title has the company and city in it"
	argumentsPath := installFakeClaude(t, initLine(""), jobFixResultLine(job.ID.String(), []string{"title"}, "Fixed: the title is Frontend Engineer."))

	var out bytes.Buffer
	if err := fixJob(context.Background(), hub.config, job.ID.String(), note, triageOptions{}, &out); err != nil {
		t.Fatalf("fix job: %v\n%s", err, out.String())
	}
	if status, _, _, _ := readAgentRun(t, hub.pool); status != store.AgentRunSucceeded {
		t.Errorf("run status = %s", status)
	}
	if !strings.Contains(out.String(), "Fixed: the title is Frontend Engineer.") {
		t.Errorf("output lacks the summary:\n%s", out.String())
	}
	recorded, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{note, "Frontend Engineer - Track&Field - São Paulo"} {
		if !strings.Contains(string(recorded), want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}

func TestAJobFixThatChangesNothingFailsWithTheReason(t *testing.T) {
	hub := startHub(t)
	job, _, _ := hub.store.AddManualJob(context.Background(), store.Actor{Kind: store.ActorOwner},
		store.ManualJobInput{Title: "Engineer", URL: "https://acme.example/1"})
	installFakeClaude(t, initLine(""), jobFixResultLine(job.ID.String(), []string{}, "The note doesn't say what's wrong."))

	var out bytes.Buffer
	err := fixJob(context.Background(), hub.config, job.ID.String(), "fix it", triageOptions{}, &out)
	if err == nil || !strings.Contains(err.Error(), "The note doesn't say what's wrong.") {
		t.Fatalf("err = %v, want the agent's reason", err)
	}
}
