package mcptools

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type fullBriefWriter interface {
	WriteFullBrief(ctx context.Context, jobID uuid.UUID) error
}

type briefReader interface {
	GetJobBrief(ctx context.Context, jobID uuid.UUID) (*store.JobBrief, error)
}

type writeFullBriefInput struct {
	JobID uuid.UUID `json:"job_id" jsonschema:"the job to brief"`
}

type writeFullBriefOutput struct {
	Match  string `json:"match"`
	Reason string `json:"reason"`
}

// AddJobBriefTools gives the owner's server write_full_brief. Agents don't
// get it: it spends the owner's Claude plan.
func AddJobBriefTools(server *mcp.Server, writer fullBriefWriter, briefs briefReader) {
	addTool(server, &mcp.Tool{
		Name: "write_full_brief",
		Description: "Have Claude write a job's full brief now: its match, the reason, and the strengths and weaknesses citing the knowledge base. " +
			"Returns when it's saved; the job's details show it over the local pre-brief. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input writeFullBriefInput) (*mcp.CallToolResult, writeFullBriefOutput, error) {
		if _, err := getOwnerActor(ctx); err != nil {
			return nil, writeFullBriefOutput{}, err
		}
		if err := writer.WriteFullBrief(ctx, input.JobID); err != nil {
			return nil, writeFullBriefOutput{}, err
		}
		brief, err := briefs.GetJobBrief(ctx, input.JobID)
		if err != nil || brief == nil {
			return nil, writeFullBriefOutput{}, err
		}
		return nil, writeFullBriefOutput{Match: brief.Match, Reason: brief.Reason}, nil
	})
}
