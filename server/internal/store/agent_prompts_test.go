package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestTheCompanyTriagePromptIsSeeded(t *testing.T) {
	hub := store.New(testdatabase.New(t))

	prompt, err := hub.GetLatestAgentPrompt(context.Background(), store.AgentRunKindCompanyTriage)
	if err != nil || prompt.Version != 1 {
		t.Fatalf("prompt = %+v, err = %v", prompt, err)
	}
	for _, placeholder := range []string{"{{company}}", "{{owner_profile}}", "{{company_dossier}}"} {
		if !strings.Contains(prompt.Body, placeholder) {
			t.Errorf("the seeded prompt has no %s", placeholder)
		}
	}
}

func TestSavingAPromptAddsTheNextVersionAndKeepsTheOld(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	original, err := hub.GetLatestAgentPrompt(ctx, store.AgentRunKindCompanyTriage)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}

	saved, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentRunKindCompanyTriage, Body: "Research {{company}} briefly.", Note: "shorter"})
	if err != nil || saved.Version != 2 || saved.Note != "shorter" {
		t.Fatalf("saved = %+v, err = %v", saved, err)
	}
	if latest, err := hub.GetLatestAgentPrompt(ctx, store.AgentRunKindCompanyTriage); err != nil || latest.ID != saved.ID {
		t.Fatalf("latest = %+v, err = %v, want version 2", latest, err)
	}
	if first, err := hub.GetAgentPrompt(ctx, store.AgentRunKindCompanyTriage, 1); err != nil || first.Body != original.Body {
		t.Fatalf("version 1 changed: err = %v", err)
	}

	var promptChanges int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM changes WHERE entity_type = 'agent_prompt' AND entity_id = $1`, saved.ID).Scan(&promptChanges); err != nil || promptChanges != 1 {
		t.Fatalf("prompt changes = %d, err = %v, want 1", promptChanges, err)
	}
}

func TestSavingAPromptRefusesAnUnknownKindOrAnEmptyBody(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()

	if _, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: "cover_letter", Body: "body"}); !errors.Is(err, store.ErrAgentPromptNotFound) {
		t.Errorf("unknown kind: err = %v", err)
	}
	if _, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentRunKindCompanyTriage, Body: "  "}); err == nil {
		t.Error("expected an error for an empty body")
	}
	if _, err := hub.GetAgentPrompt(ctx, store.AgentRunKindCompanyTriage, 99); !errors.Is(err, store.ErrAgentPromptNotFound) {
		t.Errorf("missing version: err = %v", err)
	}
}

func TestTheJobFactsPromptIsSeededWithALabelledSchema(t *testing.T) {
	prompt, err := store.New(testdatabase.New(t)).GetLatestAgentPrompt(context.Background(), store.AgentPromptKindJobFacts)
	if err != nil || prompt.Version != 1 {
		t.Fatalf("prompt = %+v, err = %v", prompt, err)
	}
	var schema struct {
		Properties map[string]struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(prompt.ResultSchema, &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	for _, fact := range []string{"summary", "technologies", "seniority", "years_of_experience", "location_restriction",
		"timezone_requirement", "visa_sponsorship", "contract_type", "pay_in_text", "languages"} {
		property, found := schema.Properties[fact]
		if !found || property.Title == "" || property.Description == "" {
			t.Errorf("fact %q is missing or unlabelled: %+v", fact, property)
		}
	}
	if strings.Contains(string(prompt.ResultSchema), `"type": [`) || strings.Contains(string(prompt.ResultSchema), `"type":[`) {
		t.Error("the seeded schema lists several types under type")
	}
}

func TestSavingAJobFactsPromptKeepsOrReplacesItsSchema(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	seeded, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)

	kept, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentPromptKindJobFacts, Body: "Record the facts."})
	if err != nil || kept.Version != 2 || !jsonEqual(t, kept.ResultSchema, seeded.ResultSchema) {
		t.Fatalf("saved without a schema = %+v, err = %v; want the seeded schema kept", kept, err)
	}
	newSchema := json.RawMessage(`{"type":"object","properties":{"stack":{"title":"Stack","description":"The stack.","type":"array","items":{"type":"string"}}}}`)
	replaced, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentPromptKindJobFacts, Body: "Record the stack.", ResultSchema: newSchema})
	if err != nil || !jsonEqual(t, replaced.ResultSchema, newSchema) {
		t.Fatalf("saved with a schema = %+v, err = %v", replaced, err)
	}

	for name, schema := range map[string]string{
		"a list":      `[1, 2]`,
		"not JSON":    `{"type":`,
		"a type list": `{"type":"object","properties":{"years":{"type":["integer","null"]}}}`,
	} {
		if _, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentPromptKindJobFacts, Body: "x", ResultSchema: json.RawMessage(schema)}); err == nil {
			t.Errorf("%s: saved, want refused", name)
		}
	}
}

func jsonEqual(t *testing.T, left, right json.RawMessage) bool {
	t.Helper()
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func TestTheSessionPromptsAreSeededAndEditable(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	for kind, placeholder := range map[string]string{store.AgentPromptKindCompanySession: "{{company_dossier}}", store.AgentPromptKindJobSession: "{{job_details}}"} {
		seeded, err := hub.GetLatestAgentPrompt(ctx, kind)
		if err != nil || !strings.Contains(seeded.Body, placeholder) || !strings.Contains(seeded.Body, "{{owner_profile}}") {
			t.Fatalf("%s = %+v, %v", kind, seeded, err)
		}
		if saved, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: kind, Body: "Be brief. " + placeholder}); err != nil || saved.Version != 2 {
			t.Fatalf("save %s = %+v, %v", kind, saved, err)
		}
	}
}

func TestASeedGivesOnlyAKindWithoutAPromptItsFirstVersion(t *testing.T) {
	database := testdatabase.New(t)
	hub := store.New(database)
	ctx := context.Background()
	if _, err := database.Exec(ctx, "DELETE FROM agent_prompts WHERE kind IN ('profile_audit', 'mail_triage')"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	stored, err := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	if err != nil {
		t.Fatalf("job facts: %v", err)
	}

	seeded, err := hub.SeedAgentPrompts(ctx, fstest.MapFS{
		"profile_audit.md":        {Data: []byte("Audit it.")},
		"mail_triage.md":          {Data: []byte("Sort it.")},
		"mail_triage.schema.json": {Data: []byte(`{"type":"object","properties":{"class":{"type":"string"}}}`)},
		"job_facts.md":            {Data: []byte("Read it again.")},
	})

	if err != nil || !slices.Equal(seeded, []string{store.AgentPromptKindMailTriage, store.AgentPromptKindProfileAudit}) {
		t.Fatalf("seeded = %v, %v", seeded, err)
	}
	mail, err := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindMailTriage)
	if err != nil || mail.Version != 1 || mail.Body != "Sort it." || !strings.Contains(string(mail.ResultSchema), `"class"`) {
		t.Fatalf("mail triage = %+v, %v", mail, err)
	}
	if facts, err := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts); err != nil || facts.ID != stored.ID {
		t.Fatalf("job facts = %+v, %v; want the stored prompt kept", facts, err)
	}
}
