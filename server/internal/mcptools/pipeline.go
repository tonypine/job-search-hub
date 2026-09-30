package mcptools

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type dismissApplicationInput struct {
	ApplicationID uuid.UUID `json:"application_id" jsonschema:"the pipeline card"`
	Note          string    `json:"note,omitempty" jsonschema:"why it isn't a good fit, in the owner's words; optional"`
}

type restoreApplicationInput struct {
	ApplicationID uuid.UUID `json:"application_id" jsonschema:"the dismissed pipeline card"`
}

func addPipelineTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name: "dismiss_application",
		Description: "Take a pipeline card off the board as not a good fit, with the owner's note, without closing it. " +
			"A card with a job dismisses the job too, so it also leaves the jobs list. Restore it with restore_application. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input dismissApplicationInput) (*mcp.CallToolResult, store.Application, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.Application{}, err
		}
		application, err := hub.DismissApplication(ctx, actor, input.ApplicationID, input.Note)
		return nil, application, err
	})
	addTool(server, &mcp.Tool{
		Name:        "restore_application",
		Description: "Put a dismissed pipeline card back on the board, in the phase it left; its job returns to the jobs list. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input restoreApplicationInput) (*mcp.CallToolResult, store.Application, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.Application{}, err
		}
		application, err := hub.RestoreApplication(ctx, actor, input.ApplicationID)
		return nil, application, err
	})
}
