package mcptools

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type addPersonInput struct {
	CompanyID  uuid.UUID `json:"company_id"`
	Name       string    `json:"name" jsonschema:"the person's full name as the source gives it"`
	RoleTitle  string    `json:"role_title,omitempty" jsonschema:"their title at the company, e.g. Engineering Manager, Payments"`
	Relevance  string    `json:"relevance" jsonschema:"one of hiring_manager, engineering_lead (an engineering manager or tech lead), engineer (a senior or staff engineer on the team), recruiter, founder, other"`
	ProfileURL string    `json:"profile_url,omitempty" jsonschema:"a public profile URL, e.g. from a team page or search result; never fetched by the hub"`
	SourceURL  string    `json:"source_url" jsonschema:"the page that names this person at this company; required"`
	Notes      string    `json:"notes,omitempty" jsonschema:"anything useful for a first message, e.g. a talk they gave or a team they lead"`
	Email      string    `json:"email,omitempty" jsonschema:"their work email, only as a public page or their own mail shows it; never guessed"`
}

type addPersonOutput struct {
	Person  store.Person `json:"person"`
	Created bool         `json:"created" jsonschema:"false when this company already had a person with this name; they are returned unchanged, except for an email they didn't have"`
}

func addPeopleTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name: "add_person",
		Description: "Store a person worth contacting at a company. Every person needs the source_url of the page " +
			"that names them there; a person without one is refused.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input addPersonInput) (*mcp.CallToolResult, addPersonOutput, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, addPersonOutput{}, err
		}
		person, created, err := hub.AddPerson(ctx, actor, store.PersonInput{
			CompanyID: input.CompanyID, Name: input.Name, RoleTitle: input.RoleTitle, Relevance: input.Relevance,
			ProfileURL: input.ProfileURL, SourceURL: input.SourceURL, Notes: input.Notes, Email: input.Email,
		})
		return nil, addPersonOutput{Person: person, Created: created}, err
	})
}
