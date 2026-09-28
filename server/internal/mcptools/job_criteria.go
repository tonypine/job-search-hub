package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type getJobCriteriaInput struct{}

func addJobCriteriaTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name:        "get_job_criteria",
		Description: "Read what makes a job worth the candidate's time: roles, search terms, stack, levels, where they can be hired from, and the pay floor.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ getJobCriteriaInput) (*mcp.CallToolResult, store.SavedJobCriteria, error) {
		saved, err := hub.GetJobCriteria(ctx)
		return nil, saved, err
	})

	addTool(server, &mcp.Tool{
		Name: "update_job_criteria",
		Description: "Replace the job criteria as a whole: read them first and send every field back. Job feeds search by " +
			"search_terms, and each job's fit is judged against the rest. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input store.JobCriteria) (*mcp.CallToolResult, store.SavedJobCriteria, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.SavedJobCriteria{}, err
		}
		saved, err := hub.SaveJobCriteria(ctx, actor, input)
		return nil, saved, err
	})
}
