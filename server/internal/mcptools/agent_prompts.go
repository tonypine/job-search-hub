package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type getAgentPromptInput struct {
	Kind    string `json:"kind" jsonschema:"the agent, e.g. company_triage"`
	Version *int   `json:"version,omitempty" jsonschema:"a past version to read; the active, latest version when absent"`
}

type updateAgentPromptInput struct {
	Kind string `json:"kind" jsonschema:"the agent, e.g. company_triage"`
	Body string `json:"body" jsonschema:"the whole new prompt; {{company}}, {{owner_profile}} and {{company_dossier}} are filled in at each run"`
	Note string `json:"note,omitempty" jsonschema:"why this version exists, e.g. what it changes"`
}

func addAgentPromptTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name:        "get_agent_prompt",
		Description: "Read an agent's prompt: the active version, or a past one by number.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getAgentPromptInput) (*mcp.CallToolResult, store.AgentPrompt, error) {
		if input.Version != nil {
			prompt, err := hub.GetAgentPrompt(ctx, input.Kind, *input.Version)
			return nil, prompt, err
		}
		prompt, err := hub.GetLatestAgentPrompt(ctx, input.Kind)
		return nil, prompt, err
	})

	addTool(server, &mcp.Tool{
		Name: "update_agent_prompt",
		Description: "Save a new version of an agent's prompt. It becomes active for the next run; earlier versions " +
			"stay readable. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input updateAgentPromptInput) (*mcp.CallToolResult, store.AgentPrompt, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.AgentPrompt{}, err
		}
		prompt, err := hub.SaveAgentPrompt(ctx, actor, input.Kind, input.Body, input.Note)
		return nil, prompt, err
	})
}
