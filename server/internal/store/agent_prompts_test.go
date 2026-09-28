package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

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

	saved, err := hub.SaveAgentPrompt(ctx, owner, store.AgentRunKindCompanyTriage, "Research {{company}} briefly.", "shorter")
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

	if _, err := hub.SaveAgentPrompt(ctx, owner, "outreach_draft", "body", ""); !errors.Is(err, store.ErrAgentPromptNotFound) {
		t.Errorf("unknown kind: err = %v", err)
	}
	if _, err := hub.SaveAgentPrompt(ctx, owner, store.AgentRunKindCompanyTriage, "  ", ""); err == nil {
		t.Error("expected an error for an empty body")
	}
	if _, err := hub.GetAgentPrompt(ctx, store.AgentRunKindCompanyTriage, 99); !errors.Is(err, store.ErrAgentPromptNotFound) {
		t.Errorf("missing version: err = %v", err)
	}
}
