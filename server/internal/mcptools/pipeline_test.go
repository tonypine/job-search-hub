package mcptools_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheOwnerMovesACardByItsJobAndAnAgentCant(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	job, _, _ := hub.store.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: "https://acme.example/1"})
	card, _, err := hub.store.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})
	if err != nil {
		t.Fatal(err)
	}
	session := connect(t, hub, ownerToken)

	yesterday := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	moved := callTool[store.Application](t, session, "move_application", map[string]any{"job_id": job.ID, "phase": "applied", "entered_on": yesterday})
	phases, _ := hub.store.ListPipelinePhases(ctx)
	var applied store.PipelinePhase
	for _, phase := range phases {
		if phase.Name == "Applied" {
			applied = phase
		}
	}
	if moved.ID != card.ID || moved.PhaseID != applied.ID || moved.PhaseEnteredAt.Format(time.DateOnly) != yesterday {
		t.Fatalf("moved = %+v, want the card in Applied since %s", moved, yesterday)
	}
	var moves int
	if err := hub.pool.QueryRow(ctx, `SELECT count(*) FROM changes WHERE entity_id = $1 AND operation = 'move'`, card.ID).Scan(&moves); err != nil || moves != 1 {
		t.Errorf("move changes = %d, %v", moves, err)
	}

	if text := callRefusedTool(t, session, "move_application", map[string]any{"application_id": card.ID, "phase": "Shortlist"}); !strings.Contains(text, "Applied") {
		t.Errorf("an unknown phase: %s, want the phases listed", text)
	}
	if text := callRefusedTool(t, session, "move_application", map[string]any{"application_id": card.ID, "phase": "Closed"}); !strings.Contains(text, "closed_reason") {
		t.Errorf("a closed phase without a reason: %s", text)
	}
	_, agentToken := startAgentRun(t, hub, time.Now().Add(time.Hour))
	if text := callRefusedTool(t, connect(t, hub, agentToken), "move_application", map[string]any{"application_id": card.ID, "phase": "Applied"}); !strings.Contains(text, "unknown tool") {
		t.Errorf("an agent moving a card: %s", text)
	}
}

func TestTheOwnerRecordsOutreachAndAFollowUpAndAnAgentCant(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, err := hub.store.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	if err != nil {
		t.Fatal(err)
	}
	session := connect(t, hub, ownerToken)

	twoDaysAgo := time.Now().AddDate(0, 0, -2).Format(time.DateOnly)
	type outreachOutput struct {
		Application store.Application `json:"application"`
		Created     bool              `json:"created"`
	}
	outreach := callTool[outreachOutput](t, session, "record_outreach", map[string]any{
		"domain": "acme.example", "note": "LinkedIn message to the engineering lead", "sent_on": twoDaysAgo,
	})
	card := outreach.Application
	if !outreach.Created || card.JobID != nil || card.CompanyID == nil || *card.CompanyID != company.ID || card.PhaseEnteredAt.Format(time.DateOnly) != twoDaysAgo {
		t.Fatalf("outreach = %+v; want a new outreach card in Applied since %s", outreach, twoDaysAgo)
	}

	followedUp := callTool[store.Application](t, session, "record_follow_up", map[string]any{"application_id": card.ID, "note": "Emailed the recruiter"})
	if followedUp.ID != card.ID || followedUp.LastFollowedUpAt == nil {
		t.Fatalf("follow-up = %+v; want the card followed up", followedUp)
	}
	var followUps int
	if err := hub.pool.QueryRow(ctx, `SELECT count(*) FROM changes WHERE entity_id = $1 AND operation = 'follow_up'`, card.ID).Scan(&followUps); err != nil || followUps != 1 {
		t.Errorf("follow-up changes = %d, %v", followUps, err)
	}

	if text := callRefusedTool(t, session, "record_outreach", map[string]any{"company_id": company.ID, "sent_on": "last week"}); !strings.Contains(text, "YYYY-MM-DD") {
		t.Errorf("a day that isn't a date: %s", text)
	}
	if text := callRefusedTool(t, session, "record_follow_up", map[string]any{"note": "?"}); !strings.Contains(text, "application_id") {
		t.Errorf("a follow-up naming no card: %s", text)
	}
	_, agentToken := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, agentToken)
	for _, tool := range []string{"record_outreach", "record_follow_up"} {
		if text := callRefusedTool(t, agent, tool, map[string]any{"company_id": company.ID, "application_id": card.ID}); !strings.Contains(text, "unknown tool") {
			t.Errorf("an agent calling %s: %s", tool, text)
		}
	}
}
