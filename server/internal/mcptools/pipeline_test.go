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
