package marketgaps

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

type noRates struct{}

func (noRates) GetRates(context.Context, string) (map[string]float64, error) { return nil, nil }

// fakePlanner plans every gap it's asked about, and keeps what it read.
type fakePlanner struct {
	inputs []string
}

func (planner *fakePlanner) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	planner.inputs = append(planner.inputs, request.User)
	return chatcompletions.Answer{Object: json.RawMessage(`{"plans":[{"gap":"graphql","kind":"portfolio","plan":"Add a GraphQL API to the scheduler side project."}]}`)}, nil
}

func addJobWithFacts(t *testing.T, hub *store.Store, title, location, facts string) {
	t.Helper()
	ctx := context.Background()
	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: title, URL: "https://acme.com/" + title, Location: location, Description: "Build."})
	if err != nil {
		t.Fatal(err)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	awaiting, _ := hub.GetJobForFacts(ctx, job.ID)
	if err := hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: job.ID, PromptID: prompt.ID, Model: "m", TextHash: awaiting.TextHash, Facts: json.RawMessage(facts)}); err != nil {
		t.Fatal(err)
	}
}

func TestARefreshFindsGapsAmongGoodFitsAndPlansThem(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveJobCriteria(ctx, owner, store.JobCriteria{Roles: []string{"Product Engineer"}, SeniorityLevels: []string{"Senior"},
		Technologies: []string{"React"}, HomeCountry: "Brazil", EligibleLocationTerms: []string{"LATAM"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: "skill", Title: "React", Body: "Built web apps in React and TypeScript.", Source: "owner"}); err != nil {
		t.Fatal(err)
	}
	good := `{"technologies":["React","GraphQL","TypeScript"],"location_restriction":"LATAM","seniority":"Senior"}`
	for _, title := range []string{"Senior Product Engineer, Growth", "Senior Product Engineer, Platform", "Senior Product Engineer, Core"} {
		addJobWithFacts(t, hub, title, "LATAM", good)
	}
	addJobWithFacts(t, hub, "Sales Manager", "US", `{"technologies":["GraphQL","Salesforce"]}`)
	planner := &fakePlanner{}

	gaps, err := NewAnalyzer(hub, planner, noRates{}).Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].Technology != "GraphQL" || gaps[0].JobCount != 3 || gaps[0].GoodFits != 3 {
		t.Fatalf("gaps = %+v, want GraphQL from the 3 good fits (React and TypeScript are known)", gaps)
	}
	if stored, _ := hub.ListMarketGaps(ctx); len(stored) != 1 || stored[0].PlanKind != "portfolio" || !strings.Contains(stored[0].Plan, "GraphQL API") {
		t.Errorf("stored = %+v, want the plan matched by its gap's name", stored)
	}
	if len(planner.inputs) != 1 || !strings.Contains(planner.inputs[0], "GraphQL: asked for by 3 of 3 good fits") {
		t.Errorf("the planner read %q", planner.inputs)
	}
	if computedAt, _ := hub.GetMarketGapsComputedAt(ctx); computedAt == nil {
		t.Error("the refresh time wasn't kept")
	}
}
