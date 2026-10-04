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

type recordFollowUpInput struct {
	ApplicationID *uuid.UUID `json:"application_id,omitempty" jsonschema:"the pipeline card; or give job_id"`
	JobID         *uuid.UUID `json:"job_id,omitempty" jsonschema:"the job whose card was followed up, when the card's id isn't known"`
	Note          string     `json:"note,omitempty" jsonschema:"what the owner did, such as 'emailed the recruiter'; optional"`
	FollowedUpOn  string     `json:"followed_up_on,omitempty" jsonschema:"the day it really happened, as YYYY-MM-DD; today when absent"`
}

type recordOutreachInput struct {
	CompanyID *uuid.UUID `json:"company_id,omitempty" jsonschema:"the company messaged; give this or domain"`
	Domain    string     `json:"domain,omitempty" jsonschema:"the company's domain or any URL on it; give this or company_id"`
	Note      string     `json:"note,omitempty" jsonschema:"who was messaged and how, such as 'LinkedIn message to the engineering lead'; optional"`
	SentOn    string     `json:"sent_on,omitempty" jsonschema:"the day the message went out, as YYYY-MM-DD; today when absent"`
}

type recordOutreachOutput struct {
	Application store.Application `json:"application"`
	Created     bool              `json:"created" jsonschema:"true when the company had no open outreach card and one was added"`
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
		enteredAt, err := parseDayOrNow(input.EnteredOn, "entered_on")
		if err != nil {
			return nil, store.Application{}, err
		}
		application, err := hub.MoveApplicationAsOf(ctx, actor, applicationID, phase.ID, input.ClosedReason, enteredAt, "")
		return nil, application, err
	})
	addTool(server, &mcp.Tool{
		Name: "record_follow_up",
		Description: "Note that the owner followed up on a pipeline card, such as a nudge to the recruiter a week after applying, " +
			"which restarts the count to the card's next follow-up. Give the card or its job. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input recordFollowUpInput) (*mcp.CallToolResult, store.Application, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.Application{}, err
		}
		applicationID, err := findApplicationToMove(ctx, hub, moveApplicationInput{ApplicationID: input.ApplicationID, JobID: input.JobID})
		if err != nil {
			return nil, store.Application{}, err
		}
		followedUpAt, err := parseDayOrNow(input.FollowedUpOn, "followed_up_on")
		if err != nil {
			return nil, store.Application{}, err
		}
		application, err := hub.RecordFollowUpAsOf(ctx, actor, applicationID, input.Note, followedUpAt, "")
		return nil, application, err
	})
	addTool(server, &mcp.Tool{
		Name: "record_outreach",
		Description: "Note that the owner messaged someone at a company cold, with no posting, so its follow-up falls due a week later like an application's. " +
			"The company's outreach card moves to Applied as of sent_on, and is added when it has none; when the card is already there or further on, " +
			"the message counts as a follow-up. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input recordOutreachInput) (*mcp.CallToolResult, recordOutreachOutput, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, recordOutreachOutput{}, err
		}
		var companyID uuid.UUID
		switch {
		case input.CompanyID != nil:
			companyID = *input.CompanyID
		case input.Domain != "":
			company, err := hub.GetCompanyByDomain(ctx, input.Domain)
			if err != nil {
				return nil, recordOutreachOutput{}, err
			}
			companyID = company.ID
		default:
			return nil, recordOutreachOutput{}, errors.New("give company_id or domain")
		}
		sentAt, err := parseDayOrNow(input.SentOn, "sent_on")
		if err != nil {
			return nil, recordOutreachOutput{}, err
		}
		application, created, err := hub.RecordOutreach(ctx, actor, companyID, input.Note, sentAt)
		return nil, recordOutreachOutput{Application: application, Created: created}, err
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

// parseDayOrNow reads a day given as YYYY-MM-DD, at its start in local
// time, or returns now when the day is absent. field names it in the error.
func parseDayOrNow(day, field string) (time.Time, error) {
	if day == "" {
		return time.Now(), nil
	}
	moment, err := time.ParseInLocation(time.DateOnly, day, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be a date as YYYY-MM-DD", field)
	}
	return moment, nil
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
