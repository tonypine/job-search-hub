package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type dismissApplicationInput struct {
	ApplicationID uuid.UUID `json:"application_id" jsonschema:"the pipeline card"`
	Note          string    `json:"note,omitempty" jsonschema:"why it isn't a good fit, in the owner's words; optional"`
}

type moveApplicationInput struct {
	ApplicationID *uuid.UUID `json:"application_id,omitempty" jsonschema:"the pipeline card; or give job_id"`
	JobID         *uuid.UUID `json:"job_id,omitempty" jsonschema:"the job whose card to move, when the card's id isn't known"`
	Phase         string     `json:"phase" jsonschema:"the phase's name, such as Applied, or its id"`
	ClosedReason  string     `json:"closed_reason,omitempty" jsonschema:"why, for a closed phase such as Rejected; required there"`
	EnteredOn     string     `json:"entered_on,omitempty" jsonschema:"the day it really happened, as YYYY-MM-DD, such as yesterday's date for 'applied yesterday'; today when absent"`
}

type restoreApplicationInput struct {
	ApplicationID uuid.UUID `json:"application_id" jsonschema:"the dismissed pipeline card"`
}

func addPipelineTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name: "move_application",
		Description: "Move a pipeline card to another phase, such as Applied once the owner has applied, which restarts its follow-up count. " +
			"Give the card or its job, and the phase by name. A closed phase needs closed_reason. An unknown phase is refused with the phases there are. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input moveApplicationInput) (*mcp.CallToolResult, store.Application, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.Application{}, err
		}
		applicationID, err := findApplicationToMove(ctx, hub, input)
		if err != nil {
			return nil, store.Application{}, err
		}
		phase, err := findPipelinePhase(ctx, hub, input.Phase)
		if err != nil {
			return nil, store.Application{}, err
		}
		if phase.IsClosed && strings.TrimSpace(input.ClosedReason) == "" {
			return nil, store.Application{}, fmt.Errorf("moving to %s closes the card, so it needs closed_reason", phase.Name)
		}
		enteredAt := time.Now()
		if input.EnteredOn != "" {
			if enteredAt, err = time.ParseInLocation(time.DateOnly, input.EnteredOn, time.Local); err != nil {
				return nil, store.Application{}, errors.New("entered_on must be a date as YYYY-MM-DD")
			}
		}
		application, err := hub.MoveApplicationAsOf(ctx, actor, applicationID, phase.ID, input.ClosedReason, enteredAt, "")
		return nil, application, err
	})
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

// findApplicationToMove returns the card the input names, or its job's card.
func findApplicationToMove(ctx context.Context, hub *store.Store, input moveApplicationInput) (uuid.UUID, error) {
	switch {
	case input.ApplicationID != nil:
		return *input.ApplicationID, nil
	case input.JobID != nil:
		details, err := hub.GetJobDetails(ctx, *input.JobID)
		if err != nil {
			return uuid.UUID{}, err
		}
		if details.Application == nil {
			return uuid.UUID{}, errors.New("the job has no pipeline card; pursue it first")
		}
		return details.Application.ID, nil
	default:
		return uuid.UUID{}, errors.New("give application_id or job_id")
	}
}

// findPipelinePhase returns the phase named, in any case, or with that id;
// an unknown one is refused with the phases there are.
func findPipelinePhase(ctx context.Context, hub *store.Store, nameOrID string) (store.PipelinePhase, error) {
	phases, err := hub.ListPipelinePhases(ctx)
	if err != nil {
		return store.PipelinePhase{}, err
	}
	names := make([]string, len(phases))
	for index, phase := range phases {
		names[index] = phase.Name
		if strings.EqualFold(phase.Name, strings.TrimSpace(nameOrID)) || phase.ID.String() == strings.TrimSpace(nameOrID) {
			return phase, nil
		}
	}
	return store.PipelinePhase{}, fmt.Errorf("no pipeline phase %q; the phases are %s", nameOrID, strings.Join(names, ", "))
}
