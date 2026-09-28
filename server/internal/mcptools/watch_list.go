package mcptools

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type watchListInput struct {
	CompanyID uuid.UUID `json:"company_id" jsonschema:"the company to put on or take off the watch list"`
}

type addToWatchListOutput struct {
	WatchedSince time.Time `json:"watched_since"`
	Added        bool      `json:"added" jsonschema:"false when the company was already on the watch list"`
}

type removeFromWatchListOutput struct {
	Removed bool `json:"removed" jsonschema:"false when the company was not on the watch list"`
}

type listWatchListOutput struct {
	Companies []store.WatchedCompany `json:"companies"`
}

func addWatchListTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name:        "add_to_watch_list",
		Description: "Put a company on the watch list of target companies.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input watchListInput) (*mcp.CallToolResult, addToWatchListOutput, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, addToWatchListOutput{}, err
		}
		watchedSince, added, err := hub.AddToWatchList(ctx, actor, input.CompanyID)
		return nil, addToWatchListOutput{WatchedSince: watchedSince, Added: added}, err
	})

	addTool(server, &mcp.Tool{
		Name:        "remove_from_watch_list",
		Description: "Take a company off the watch list. The company and its dossier are kept.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input watchListInput) (*mcp.CallToolResult, removeFromWatchListOutput, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, removeFromWatchListOutput{}, err
		}
		removed, err := hub.RemoveFromWatchList(ctx, actor, input.CompanyID)
		return nil, removeFromWatchListOutput{Removed: removed}, err
	})

	addTool(server, &mcp.Tool{
		Name:        "list_watch_list",
		Description: "List the watched companies, most recently added first.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listWatchListOutput, error) {
		companies, err := hub.ListWatchList(ctx)
		return nil, listWatchListOutput{Companies: companies}, err
	})
}
