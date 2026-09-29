package mcptools_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestAgentsListFilesAndReadTheirTextButNeverTheFile(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, _ := hub.store.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	resume, _, err := hub.store.SaveArtifact(ctx, owner, store.NewArtifact{
		Kind: store.ArtifactResume, Name: "resume.pdf", ContentType: "application/pdf", Content: []byte("%PDF-raw"), Text: "Sam Example, senior engineer.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := hub.store.SaveArtifact(ctx, owner, store.NewArtifact{
		CompanyID: &company.ID, Kind: store.ArtifactSavedPage, Name: "team.html", Content: []byte("<p>Team</p>"), Text: "Team",
	}); err != nil {
		t.Fatal(err)
	}
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)

	owners := callTool[struct {
		Artifacts []store.Artifact `json:"artifacts"`
	}](t, agent, "list_artifacts", map[string]any{})
	companys := callTool[struct {
		Artifacts []store.Artifact `json:"artifacts"`
	}](t, agent, "list_artifacts", map[string]any{"company_domain": "acme.com"})
	if len(owners.Artifacts) != 1 || owners.Artifacts[0].Name != "resume.pdf" || len(companys.Artifacts) != 1 || companys.Artifacts[0].Name != "team.html" {
		t.Fatalf("owner's = %+v, company's = %+v", owners.Artifacts, companys.Artifacts)
	}
	read := callTool[map[string]any](t, agent, "get_artifact_text", map[string]any{"id": resume.ID.String()})
	if read["text"] != "Sam Example, senior engineer." {
		t.Fatalf("text = %+v", read)
	}
	for key, value := range read {
		if text, isText := value.(string); isText && strings.Contains(text, "%PDF") {
			t.Errorf("the raw file reached the agent in %q", key)
		}
	}
}
