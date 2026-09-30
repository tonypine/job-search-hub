package mcptools

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/modelwork"
)

type readJobFactsNowInput struct {
	JobID uuid.UUID `json:"job_id" jsonschema:"the job whose facts to read"`
}

type readJobFactsNowOutput struct {
	Queued bool `json:"queued"`
}

type noInput struct{}

// AddModelWorkTools gives the owner's server the tools that show the hub's
// model work, pause and resume it, and read one job's facts now. Agents
// don't get them.
func AddModelWorkTools(server *mcp.Server, controls *modelwork.Controls) {
	addTool(server, &mcp.Tool{
		Name:        "get_model_work",
		Description: "Show the hub's model work: whether background work is paused, the call running, the calls waiting, the local model runtime's state and model, and how many jobs wait for facts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, modelwork.Status, error) {
		status, err := controls.GetStatus(ctx)
		return nil, status, err
	})
	for name, paused := range map[string]bool{"pause_model_work": true, "resume_model_work": false} {
		description := "Pause the hub's background model work (job facts and sorting); the running call finishes and the model unloads. Runs the owner dispatches still run. Owner only."
		if !paused {
			description = "Resume the hub's background model work. Owner only."
		}
		addTool(server, &mcp.Tool{Name: name, Description: description},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, modelwork.Status, error) {
				actor, err := getOwnerActor(ctx)
				if err != nil {
					return nil, modelwork.Status{}, err
				}
				if err := controls.SetPaused(ctx, actor, paused); err != nil {
					return nil, modelwork.Status{}, err
				}
				status, err := controls.GetStatus(ctx)
				return nil, status, err
			})
	}
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
