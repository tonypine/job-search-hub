package mcptools

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type listProfileEntriesInput struct {
	Kind          string `json:"kind,omitempty" jsonschema:"only entries of this kind: role, case, skill, education, project, preference or fact"`
	ConfirmedOnly bool   `json:"confirmed_only,omitempty" jsonschema:"only the entries the candidate confirmed, the ones that may speak for them"`
}

type profileEntriesOutput struct {
	Entries []store.ProfileEntry `json:"entries"`
}

type saveProfileEntryInput struct {
	ID *uuid.UUID `json:"id,omitempty" jsonschema:"the entry to replace; leave out to add a new one"`
	store.ProfileEntryInput
}

type profileEntryIDsInput struct {
	IDs []uuid.UUID `json:"ids" jsonschema:"the entries' ids"`
}

type profileEntryIDInput struct {
	ID uuid.UUID `json:"id" jsonschema:"the entry's id"`
}

type deletedOutput struct {
	Deleted bool `json:"deleted"`
}

func addProfileEntryTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name: "list_profile_entries",
		Description: "List the knowledge base of the candidate's experience: roles held (organization, title, months), cases of work " +
			"within a role (what, how, stack, outcome), skills, education, projects, preferences and other facts, each with its source. " +
			"Only confirmed entries speak for the candidate; use those for anything they will send.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listProfileEntriesInput) (*mcp.CallToolResult, profileEntriesOutput, error) {
		filter := store.ProfileEntryFilter{Kind: input.Kind}
		if input.ConfirmedOnly {
			confirmed := true
			filter.Confirmed = &confirmed
		}
		entries, err := hub.ListProfileEntries(ctx, filter)
		return nil, profileEntriesOutput{Entries: entries}, err
	})

	addTool(server, &mcp.Tool{
		Name: "save_profile_entry",
		Description: "Add an entry to the candidate's knowledge base, or replace one by id. Kinds: role (organization, title, " +
			"start_month and end_month as YYYY-MM), case (a piece of work: title, body with what and how, skills, outcome, and role_id " +
			"of its role), skill, education, project, preference, fact. Source is cv, linkedin, interview or owner, with source_detail " +
			"saying where exactly. Record only what a source says or the candidate told you; never infer or embellish. A saved entry " +
			"waits for the candidate to confirm it, and a change by an agent unconfirms it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveProfileEntryInput) (*mcp.CallToolResult, store.ProfileEntry, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, store.ProfileEntry{}, err
		}
		saved, err := hub.SaveProfileEntry(ctx, actor, input.ID, input.ProfileEntryInput)
		return nil, saved, err
	})

	addTool(server, &mcp.Tool{
		Name:        "confirm_profile_entries",
		Description: "Confirm knowledge base entries as the candidate's own account, only when they say each one is right. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input profileEntryIDsInput) (*mcp.CallToolResult, profileEntriesOutput, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, profileEntriesOutput{}, err
		}
		confirmed, err := hub.ConfirmProfileEntries(ctx, actor, input.IDs)
		return nil, profileEntriesOutput{Entries: confirmed}, err
	})

	addTool(server, &mcp.Tool{
		Name:        "delete_profile_entry",
		Description: "Delete a knowledge base entry the candidate says is wrong. A role's cases stay, no longer tied to it. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input profileEntryIDInput) (*mcp.CallToolResult, deletedOutput, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, deletedOutput{}, err
		}
		err = hub.DeleteProfileEntry(ctx, actor, input.ID)
		return nil, deletedOutput{Deleted: err == nil}, err
	})
}
