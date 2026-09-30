package mcptools

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type setJobCompanyInput struct {
	JobID     uuid.UUID `json:"job_id"`
	CompanyID uuid.UUID `json:"company_id" jsonschema:"a company already in the hub; create it first with create_company"`
}

type dismissJobsInput struct {
	JobIDs []uuid.UUID `json:"job_ids" jsonschema:"the jobs to dismiss"`
	Reason string      `json:"reason,omitempty" jsonschema:"why, in the owner's words; optional"`
}

type restoreJobsInput struct {
	JobIDs []uuid.UUID `json:"job_ids" jsonschema:"the dismissed jobs to bring back"`
}

type decideJobInput struct {
	JobID    uuid.UUID `json:"job_id"`
	Decision string    `json:"decision" jsonschema:"pursue, skip or later"`
	Reason   string    `json:"reason,omitempty" jsonschema:"why, in the owner's words; a skip keeps it"`
}

type jobsOutput struct {
	Jobs []store.Job `json:"jobs"`
}

func addJobTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name: "decide_job",
		Description: "Record the owner's decision on a job: pursue puts it on the pipeline, skip dismisses it with the reason, " +
			"and later only records it. Pursuing or leaving for later a dismissed job restores it. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input decideJobInput) (*mcp.CallToolResult, store.JobDecision, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.JobDecision{}, err
		}
		decision, err := hub.DecideJob(ctx, actor, input.JobID, input.Decision, input.Reason)
		return nil, decision, err
	})
	addTool(server, &mcp.Tool{
		Name: "dismiss_job",
		Description: "Dismiss one or more jobs the owner doesn't want to see again, with an optional reason. They leave the jobs list, " +
			"stay dismissed when their board lists them again, and can be restored with restore_job. An unknown id dismisses nothing. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input dismissJobsInput) (*mcp.CallToolResult, jobsOutput, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, jobsOutput{}, err
		}
		jobs, err := hub.DismissJobs(ctx, actor, input.JobIDs, strings.TrimSpace(input.Reason))
		return nil, jobsOutput{Jobs: jobs}, err
	})
	addTool(server, &mcp.Tool{
		Name:        "restore_job",
		Description: "Bring dismissed jobs back to the jobs list. An unknown id restores nothing. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input restoreJobsInput) (*mcp.CallToolResult, jobsOutput, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, jobsOutput{}, err
		}
		jobs, err := hub.RestoreJobs(ctx, actor, input.JobIDs)
		return nil, jobsOutput{Jobs: jobs}, err
	})
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
