package interviewpacks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// fakeCoach cites E1 and an E9 that doesn't exist, and keeps what it read.
type fakeCoach struct {
	systems []string
	inputs  []string
}

func (coach *fakeCoach) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	coach.systems = append(coach.systems, request.System)
	coach.inputs = append(coach.inputs, request.User)
	return chatcompletions.Answer{Model: "local-27b", Object: json.RawMessage(`{"questions":[{"question":"Tell us about a scheduler you built.",
		"reason":"The posting asks for scheduling experience.","stories":["E1","E9"],"talking_points":"No-shows fell."}],
		"role_gaps":[{"gap":"GraphQL","honest_answer":"Say you've used REST, and how you'd learn it."}]}`)}, nil
}

func TestAPursuedJobGetsAPackCitingOnlyConfirmedCases(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	confirmedCase, _ := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: "case", Title: "Built the scheduler", Outcome: "Cut no-shows", Source: "owner"})
	if _, err := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: "case", Title: "Unconfirmed story", Source: "interview"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.ConfirmProfileEntries(ctx, owner, []uuid.UUID{confirmedCase.ID}); err != nil {
		t.Fatal(err)
	}
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Product Engineer", URL: "https://acme.com/1", Description: "Scheduling and GraphQL."})
	if _, err := hub.DecideJob(ctx, owner, job.ID, store.JobDecisionPursue, ""); err != nil {
		t.Fatal(err)
	}
	if err := hub.SaveMarketGaps(ctx, []store.MarketGap{{Technology: "GraphQL", JobCount: 3, GoodFits: 9, JobIDs: []uuid.UUID{job.ID}}}); err != nil {
		t.Fatal(err)
	}
	coach := &fakeCoach{}
	preparer := NewPreparer(hub, coach)

	if summary, err := preparer.PrepareOnce(ctx); err != nil || summary.Prepared != 1 {
		t.Fatalf("pass = %+v, %v", summary, err)
	}
	if !strings.Contains(coach.systems[0], "Built the scheduler") || strings.Contains(coach.systems[0], "Unconfirmed story") {
		t.Errorf("the knowledge base given:\n%s", coach.systems[0])
	}
	if !strings.Contains(coach.inputs[0], "Scheduling and GraphQL.") || !strings.Contains(coach.inputs[0], "- GraphQL") {
		t.Errorf("the input:\n%s", coach.inputs[0])
	}
	stored, err := hub.GetInterviewPack(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var pack Pack
	if err := json.Unmarshal(stored.Pack, &pack); err != nil {
		t.Fatal(err)
	}
	stories := pack.Questions[0].Stories
	if len(stories) != 1 || stories[0].EntryID != confirmedCase.ID || stories[0].Title != "Built the scheduler" {
		t.Errorf("stories = %+v, want only the confirmed case (E9 names none)", stories)
	}
	if len(pack.RoleGaps) != 1 || pack.RoleGaps[0].Gap != "GraphQL" {
		t.Errorf("role gaps = %+v", pack.RoleGaps)
	}

	if summary, _ := preparer.PrepareOnce(ctx); summary.Prepared != 0 {
		t.Errorf("a current pack was prepared again: %+v", summary)
	}
	if _, err := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: "skill", Title: "Go", Source: "owner"}); err != nil {
		t.Fatal(err)
	}
	if summary, _ := preparer.PrepareOnce(ctx); summary.Prepared != 1 {
		t.Errorf("after the knowledge base changed: %+v, want the pack prepared again", summary)
	}
}
