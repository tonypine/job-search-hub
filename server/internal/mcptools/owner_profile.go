package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type updateOwnerProfileInput struct {
	Body string `json:"body" jsonschema:"the whole profile as markdown: background, skills, positioning, location and timezone, contract and rate preferences, links"`
}

func addOwnerProfileTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name:        "get_owner_profile",
		Description: "Read the profile of the candidate the hub works for.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, store.OwnerProfile, error) {
		profile, err := hub.GetOwnerProfile(ctx)
		return nil, profile, err
	})

	addTool(server, &mcp.Tool{
		Name:        "update_owner_profile",
		Description: "Replace the candidate's profile. Agent runs read it as context. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input updateOwnerProfileInput) (*mcp.CallToolResult, store.OwnerProfile, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.OwnerProfile{}, err
		}
		profile, err := hub.SaveOwnerProfile(ctx, actor, input.Body)
		return nil, profile, err
	})
}
