package mcptools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type getAgentPromptInput struct {
	Kind    string `json:"kind" jsonschema:"the prompt: company_triage or job_facts"`
	Version *int   `json:"version,omitempty" jsonschema:"a past version to read; the active, latest version when absent"`
}

type updateAgentPromptInput struct {
	Kind         string         `json:"kind" jsonschema:"the prompt: company_triage or job_facts"`
	Body         string         `json:"body" jsonschema:"the whole new prompt; in company_triage, {{company}}, {{owner_profile}} and {{company_dossier}} are filled in at each run"`
	ResultSchema map[string]any `json:"result_schema,omitempty" jsonschema:"the JSON schema the answer must match; for job_facts, one property per fact, each with a title and a description; the previous version's schema is kept when absent"`
	Note         string         `json:"note,omitempty" jsonschema:"why this version exists, e.g. what it changes"`
}

func addAgentPromptTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name:        "get_agent_prompt",
		Description: "Read a prompt, with the schema its answer must match: the active version, or a past one by number.",
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
		Description: "Save a new version of a prompt, optionally with a new answer schema. It becomes active for the next run; " +
			"earlier versions stay readable. Saving a new job_facts version makes the hub read every open job's facts again. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input updateAgentPromptInput) (*mcp.CallToolResult, store.AgentPrompt, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.AgentPrompt{}, err
		}
		newPrompt := store.NewAgentPrompt{Kind: input.Kind, Body: input.Body, Note: input.Note}
		if input.ResultSchema != nil {
			if newPrompt.ResultSchema, err = json.Marshal(input.ResultSchema); err != nil {
				return nil, store.AgentPrompt{}, err
			}
		}
		prompt, err := hub.SaveAgentPrompt(ctx, actor, newPrompt)
		return nil, prompt, err
	})
}
