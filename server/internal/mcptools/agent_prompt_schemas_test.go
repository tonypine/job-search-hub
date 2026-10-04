package mcptools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/mcptools"
	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// The schemas are read off a server that has no store behind it: listing
// the tools never reaches the database.
func TestThePromptToolsNameEveryKindAndItsPlaceholders(t *testing.T) {
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := mcptools.NewServer(nil, stubJobBoards{}, nil, noRates{}).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("serve: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	descriptions := map[string]map[string]string{}
	for _, tool := range listed.Tools {
		encoded, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: encode schema: %v", tool.Name, err)
		}
		var schema struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("%s: decode schema: %v", tool.Name, err)
		}
		descriptions[tool.Name] = map[string]string{}
		for name, property := range schema.Properties {
			descriptions[tool.Name][name] = property.Description
		}
	}

	for _, tool := range []string{"get_agent_prompt", "update_agent_prompt"} {
		for _, kind := range store.AgentPromptKinds {
			if !strings.Contains(descriptions[tool]["kind"], kind) {
				t.Errorf("%s kind = %q, missing %s", tool, descriptions[tool]["kind"], kind)
			}
		}
	}
	body := descriptions["update_agent_prompt"]["body"]
	for _, info := range prompts.Kinds {
		if !strings.Contains(body, info.Kind) {
			t.Errorf("body = %q, missing %s", body, info.Kind)
		}
		for _, placeholder := range info.Placeholders {
			if !strings.Contains(body, placeholder) {
				t.Errorf("body = %q, missing %s's %s", body, info.Kind, placeholder)
			}
		}
	}
	if !strings.Contains(body, "in job_session, {{owner_profile}}, {{owner_voice}}, {{application_answers}}, {{job_details}} and {{company_dossier}}") {
		t.Errorf("body = %q, want job_session's placeholders listed under it", body)
	}
}
