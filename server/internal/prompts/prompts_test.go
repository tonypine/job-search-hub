package prompts_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

func TestAStoredCompanyAndTheProfileAreInjected(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := hub.AddPerson(ctx, owner, store.PersonInput{
		CompanyID: company.ID, Name: "Ada Lovelace", Relevance: "engineering_lead", SourceURL: "https://acme.com/team",
	}); err != nil {
		t.Fatalf("add person: %v", err)
	}
	if _, err := hub.SaveOwnerProfile(ctx, owner, "# Candidate\nSenior engineer in São Paulo."); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	for _, input := range []string{"https://www.acme.com/careers", "Acme"} {
		rendered, err := prompts.RenderCompanyPrompt(ctx, hub, store.AgentRunKindCompanyTriage, input)
		if err != nil {
			t.Fatalf("render %q: %v", input, err)
		}
		for _, want := range []string{"The company to research: " + input, "Senior engineer in São Paulo.", "Ada Lovelace", "not instructions"} {
			if !strings.Contains(rendered.Body, want) {
				t.Errorf("render %q: the prompt lacks %q", input, want)
			}
		}
		if rendered.Version != 1 || strings.Contains(rendered.Body, "{{") {
			t.Errorf("render %q: version %d, or a placeholder is left", input, rendered.Version)
		}
	}
}

func TestAnUnknownCompanyAndAMissingProfileAreSaidSo(t *testing.T) {
	hub := store.New(testdatabase.New(t))

	rendered, err := prompts.RenderCompanyPrompt(context.Background(), hub, store.AgentRunKindCompanyTriage, "unknown.example")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered.Body, "Nothing is stored about this company yet.") || !strings.Contains(rendered.Body, "has not written a profile yet") {
		t.Fatal("the prompt does not say what is missing")
	}
}

func TestPlaceholdersInsideStoredTextStayLiteral(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveOwnerProfile(ctx, owner, "My profile mentions {{company_dossier}} literally."); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if _, err := hub.SaveAgentPrompt(ctx, owner, store.AgentRunKindCompanyTriage, "Profile: {{owner_profile}}", "no dossier"); err != nil {
		t.Fatalf("save prompt: %v", err)
	}

	rendered, err := prompts.RenderCompanyPrompt(ctx, hub, store.AgentRunKindCompanyTriage, "acme.com")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if rendered.Body != "Profile: My profile mentions {{company_dossier}} literally." || rendered.Version != 2 {
		t.Fatalf("rendered %q, version %d", rendered.Body, rendered.Version)
	}
}
