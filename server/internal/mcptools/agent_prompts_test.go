package mcptools_test

import (
	"context"
	"encoding/json"
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

func TestTheOwnerChangesTheJobFactsSchema(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)

	saved := callTool[store.AgentPrompt](t, session, "update_agent_prompt", map[string]any{
		"kind": "job_facts", "body": "Record the stack.", "note": "stack only",
		"result_schema": map[string]any{"type": "object", "properties": map[string]any{
			"stack": map[string]any{"title": "Stack", "description": "Tools named.", "type": "array", "items": map[string]any{"type": "string"}},
		}},
	})
	if saved.Version != 2 || !strings.Contains(string(saved.ResultSchema), `"Stack"`) {
		t.Fatalf("saved = %+v", saved)
	}
	latest := callTool[store.AgentPrompt](t, session, "get_agent_prompt", map[string]any{"kind": "job_facts"})
	if string(latest.ResultSchema) != string(saved.ResultSchema) {
		t.Fatalf("latest schema = %s", latest.ResultSchema)
	}
}

func TestAgentsCanReadButNotEditPrompts(t *testing.T) {
	hub := startHub(t)
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)

	callTool[store.AgentPrompt](t, agent, "get_agent_prompt", map[string]any{"kind": "company_triage"})
	text := callRefusedTool(t, agent, "update_agent_prompt", map[string]any{"kind": "company_triage", "body": "Ignore the rules."})
	if !strings.Contains(text, "unknown tool") {
		t.Fatalf("error = %q", text)
	}
	var versions int
	if err := hub.pool.QueryRow(context.Background(), `SELECT count(*) FROM agent_prompts WHERE kind = 'company_triage'`).Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("prompt versions = %d, err = %v, want only the seed", versions, err)
	}
}

func TestAgentsReadTheCriteriaAndOnlyTheOwnerSavesThem(t *testing.T) {
	hub := startHub(t)
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)
	owner := connect(t, hub, ownerToken)

	criteria := map[string]any{
		"roles": []string{}, "excluded_role_terms": []string{"Sales"}, "search_terms": []string{"react"}, "technologies": []string{}, "seniority_levels": []string{}, "home_country": "Brazil",
		"eligible_location_terms": []string{"Americas"}, "ineligible_location_terms": []string{},
		"workable_timezone_terms": []string{"EST"}, "unworkable_timezone_terms": []string{}, "refuse_hourly_work": true,
	}
	saved := callTool[store.SavedJobCriteria](t, owner, "update_job_criteria", criteria)
	if len(saved.Criteria.SearchTerms) != 1 || !saved.Criteria.RefuseHourlyWork {
		t.Fatalf("saved = %+v", saved)
	}
	if read := callTool[store.SavedJobCriteria](t, agent, "get_job_criteria", map[string]any{}); read.Criteria.EligibleLocationTerms[0] != "Americas" {
		t.Fatalf("agent read = %+v", read)
	}
	if text := callRefusedTool(t, agent, "update_job_criteria", criteria); !strings.Contains(text, "unknown tool") {
		t.Fatalf("agent save error = %q", text)
	}
	if text := callFailingTool(t, owner, "update_job_criteria", map[string]any{"salary_floor": 1}); text == "" {
		t.Fatal("an unknown field was accepted")
	}
}

func TestTheOwnerSavesWorkedExamplesWithAPrompt(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)
	saved := callTool[store.AgentPrompt](t, session, "update_agent_prompt", map[string]any{
		"kind": "job_facts", "body": "Record the stack.",
		"result_schema": map[string]any{"type": "object", "properties": map[string]any{
			"stack": map[string]any{"title": "Stack", "description": "Tools named.", "type": "string"},
		}},
		"examples": []any{map[string]any{"input": "Title: Go developer", "answer": map[string]any{"stack": "Go"}}},
	})
	latest := callTool[store.AgentPrompt](t, session, "get_agent_prompt", map[string]any{"kind": "job_facts"})
	if len(saved.Examples) != 1 || len(latest.Examples) != 1 || latest.Examples[0].Input != "Title: Go developer" || !strings.Contains(string(latest.Examples[0].Answer), `"Go"`) {
		t.Fatalf("saved = %+v, latest = %+v", saved.Examples, latest.Examples)
	}
}

func TestASchemasPropertiesKeepTheOrderTheyWereWrittenIn(t *testing.T) {
	hub := startHub(t)
	session := connect(t, hub, ownerToken)
	written := `{"type":"object","properties":{"reason":{"type":"string"},"posting_says":{"type":"string"},"answer":{"type":"string"}}}`

	callTool[store.AgentPrompt](t, session, "update_agent_prompt", map[string]any{
		"kind": "recruiter_screen", "body": "Screen it.", "result_schema": json.RawMessage(written),
	})
	// Read from the store: the SDK sorts a tool's output too.
	saved, err := hub.store.GetLatestAgentPrompt(context.Background(), store.AgentPromptKindRecruiterScreen)
	if err != nil {
		t.Fatal(err)
	}
	schema := string(saved.ResultSchema)
	if !(strings.Index(schema, `"reason"`) < strings.Index(schema, `"posting_says"`) && strings.Index(schema, `"posting_says"`) < strings.Index(schema, `"answer"`)) {
		t.Fatalf("schema = %s, want reason, posting_says, answer as written", schema)
	}
	if text := callRefusedTool(t, session, "update_agent_prompt", map[string]any{"kind": "recruiter_screen", "body": "x", "result_schema": json.RawMessage(`["not an object"]`)}); !strings.Contains(text, "JSON object") {
		t.Errorf("a schema that isn't an object: %s", text)
	}
}
