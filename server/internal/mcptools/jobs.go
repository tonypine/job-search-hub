package mcptools

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type setJobCompanyInput struct {
	JobID     uuid.UUID `json:"job_id"`
	CompanyID uuid.UUID `json:"company_id" jsonschema:"a company already in the hub; create it first with create_company"`
}

func addJobTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name: "set_job_company",
		Description: "Tie a job added by hand, or found in a feed, to its company; its pipeline card follows. " +
			"A job from a company's own board already has its company and is refused.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input setJobCompanyInput) (*mcp.CallToolResult, store.Job, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, store.Job{}, err
		}
		job, err := hub.SetJobCompany(ctx, actor, input.JobID, input.CompanyID)
		return nil, job, err
	})
}
