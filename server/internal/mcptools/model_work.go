package mcptools

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type readJobFactsNowInput struct {
	JobID uuid.UUID `json:"job_id" jsonschema:"the job whose facts to read"`
}

type readJobFactsNowOutput struct {
	Queued bool `json:"queued"`
}

type noInput struct{}

type listRunnableKindsInput struct {
	JobID     *uuid.UUID `json:"job_id,omitempty" jsonschema:"the job whose runs to list; give it or company_id"`
	CompanyID *uuid.UUID `json:"company_id,omitempty" jsonschema:"the company whose runs to list; give it or job_id"`
}

type listRunnableKindsOutput struct {
	Kinds []modelwork.RunnableKind `json:"kinds"`
}

type startRunInput struct {
	Kind      string     `json:"kind" jsonschema:"the kind of run, as list_runnable_kinds names it, such as job_brief_full or company_triage"`
	JobID     *uuid.UUID `json:"job_id,omitempty" jsonschema:"the job to run it for, for a job's kind"`
	CompanyID *uuid.UUID `json:"company_id,omitempty" jsonschema:"the company to run it for, for a company's kind"`
	// ProviderID and Model choose the model for this run only.
	ProviderID *uuid.UUID `json:"provider_id,omitempty" jsonschema:"the model provider for this run only; leave it out for the kind's own"`
	Model      string     `json:"model,omitempty" jsonschema:"the model for this run only, on provider_id or the kind's provider; leave it out for the kind's own"`
	Note       string     `json:"note,omitempty" jsonschema:"for job_fix only: what's wrong with the job's details"`
}

// getSubject is the one subject the input names.
func getSubject(jobID, companyID *uuid.UUID) (string, uuid.UUID, error) {
	switch {
	case (jobID == nil) == (companyID == nil):
		return "", uuid.UUID{}, errors.New("give job_id or company_id, not both")
	case jobID != nil:
		return store.RunSubjectJob, *jobID, nil
	default:
		return store.RunSubjectCompany, *companyID, nil
	}
}

// AddModelWorkTools gives the owner's server the tools that show the hub's
// model work, pause and resume it, list and start the runs a job or a
// company can have, and read one job's facts now. Agents don't get them.
func AddModelWorkTools(server *mcp.Server, controls *modelwork.Controls) {
	addTool(server, &mcp.Tool{
		Name:        "get_model_work",
		Description: "Show the hub's model work: whether every background kind is paused and which kinds are, the call running, the calls waiting, the local model runtime's state and model, and how many jobs wait for facts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, modelwork.Status, error) {
		status, err := controls.GetStatus(ctx)
		return nil, status, err
	})
	for name, paused := range map[string]bool{"pause_model_work": true, "resume_model_work": false} {
		description := "Pause every kind of background work the hub starts by itself, model calls and Claude runs alike; the running runs finish and the model unloads. Runs the owner dispatches still run. Owner only."
		if !paused {
			description = "Resume every kind of background work the hub starts by itself. Owner only."
		}
		addTool(server, &mcp.Tool{Name: name, Description: description},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, modelwork.Status, error) {
				actor, err := getOwnerActor(ctx)
				if err != nil {
					return nil, modelwork.Status{}, err
				}
				if err := controls.SetAllPaused(ctx, actor, paused); err != nil {
					return nil, modelwork.Status{}, err
				}
				status, err := controls.GetStatus(ctx)
				return nil, status, err
			})
	}
	addTool(server, &mcp.Tool{
		Name: "list_runnable_kinds",
		Description: "List the kinds of run a job or a company can have, such as its facts, briefs, tailored CV, recruiter screen, " +
			"interview pack and fix for a job, or research and finding jobs for a company: each with the model it runs on, its latest run " +
			"for the subject, and whether it can run now, or why not yet. Owner only.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listRunnableKindsInput) (*mcp.CallToolResult, listRunnableKindsOutput, error) {
		if _, err := getOwnerActor(ctx); err != nil {
			return nil, listRunnableKindsOutput{}, err
		}
		subjectType, subjectID, err := getSubject(input.JobID, input.CompanyID)
		if err != nil {
			return nil, listRunnableKindsOutput{}, err
		}
		kinds, err := controls.ListRunnableKinds(ctx, subjectType, subjectID)
		return nil, listRunnableKindsOutput{Kinds: kinds}, err
	})
	addTool(server, &mcp.Tool{
		Name: "start_run",
		Description: "Start one kind of run for a job or a company now, ahead of background work and even while its kind is paused, " +
			"on its kind's model or on a provider and model chosen for this run only; the kind's model stays as it was. Returns once " +
			"queued, with the id the run is recorded under: a model run's record appears when it ends, and an agent run, which runs on " +
			"the Mac, once the Mac starts it. A kind that can't run yet says why and starts nothing. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input startRunInput) (*mcp.CallToolResult, modelwork.StartedRun, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, modelwork.StartedRun{}, err
		}
		subjectType, subjectID, err := getSubject(input.JobID, input.CompanyID)
		if err != nil {
			return nil, modelwork.StartedRun{}, err
		}
		started, err := controls.StartRun(ctx, actor, modelwork.StartRequest{
			Kind: input.Kind, SubjectType: subjectType, SubjectID: subjectID, ProviderID: input.ProviderID, Model: input.Model, Note: input.Note,
		})
		return nil, started, err
	})
	addTool(server, &mcp.Tool{
		Name:        "read_job_facts_now",
		Description: "Read one job's facts with the latest job_facts prompt, ahead of background work and even while it's paused. Returns once queued; the job's facts change when the run ends. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input readJobFactsNowInput) (*mcp.CallToolResult, readJobFactsNowOutput, error) {
		if _, err := getOwnerActor(ctx); err != nil {
			return nil, readJobFactsNowOutput{}, err
		}
		if err := controls.DispatchJobFacts(ctx, input.JobID); err != nil {
			return nil, readJobFactsNowOutput{}, err
		}
		return nil, readJobFactsNowOutput{Queued: true}, nil
	})
}
