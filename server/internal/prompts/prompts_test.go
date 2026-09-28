package prompts_test

import (
	"context"
	"strings"
	"testing"
	"time"

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
	if _, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentRunKindCompanyTriage, Body: "Profile: {{owner_profile}}", Note: "no dossier"}); err != nil {
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

func TestDraftsReadTheOwnersRepliesToRecruitersButNotTheirPrivateChats(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	const ownerURL, recruiterURL, friendURL = "https://www.linkedin.com/in/owner", "https://www.linkedin.com/in/recruiter", "https://www.linkedin.com/in/friend"
	at := time.Date(2025, 2, 13, 12, 0, 0, 0, time.UTC)
	message := func(conversation, from, to, content string, minutes int) store.NewLinkedInMessage {
		return store.NewLinkedInMessage{
			ConversationID: conversation, SenderName: from, SenderProfileURL: from, RecipientProfileURLs: []string{to},
			SentAt: at.Add(time.Duration(minutes) * time.Minute), Content: content,
		}
	}
	if _, err := hub.ImportLinkedInMessages(ctx, owner, []store.NewLinkedInMessage{
		message("started-by-owner", ownerURL, friendURL, "Hi, great to connect with you here and see what you're building.", 0),
		message("recruiter", recruiterURL, ownerURL, "Hi, I have a senior role that would be a great fit for you.", 1),
		message("recruiter", ownerURL, recruiterURL, "Hi Sam, thanks for getting in touch. I would like to hear more about the team.", 2),
		message("friend", friendURL, ownerURL, "E aí, tudo bem? Quanto tempo!", 3),
		message("friend", ownerURL, friendURL, "Tudo bem sim, e você? Acabei de sair da empresa, então estou na procura de novo.", 4),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}
	conversations, err := hub.ListConversationsAwaitingClassification(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, conversation := range conversations {
		class := "known_person"
		if conversation.StartedByURL == recruiterURL {
			class = "recruiter_outreach"
		}
		if err := hub.SaveConversationClassification(ctx, conversation.ID, store.ConversationClassification{Class: class, ClassifiedBy: store.ClassifiedByRule}); err != nil {
			t.Fatalf("classify: %v", err)
		}
	}

	if _, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{
		Kind: store.AgentPromptKindJobSession, Body: "## How I write\n\n{{owner_voice}}\n\n## The job\n\n{{job_details}}",
	}); err != nil {
		t.Fatalf("save the prompt: %v", err)
	}

	rendered, err := prompts.RenderJobSessionContext(ctx, hub, map[string]string{"title": "Senior Engineer"}, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered.Body, "## How I write") || !strings.Contains(rendered.Body, "I would like to hear more about the team.") {
		t.Errorf("the session lacks the owner's reply to the recruiter:\n%s", rendered.Body)
	}
	if strings.Contains(rendered.Body, "estou na procura") || strings.Contains(rendered.Body, "{{") {
		t.Errorf("the session carries a private chat, or a placeholder is left:\n%s", rendered.Body)
	}
}
