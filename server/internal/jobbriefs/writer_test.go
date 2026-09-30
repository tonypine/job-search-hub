package jobbriefs_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/jobbriefs"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// fakeModel briefs every job as a possible match whose one strength cites
// E2 and a reference the knowledge base doesn't have.
type fakeModel struct {
	requests []chatcompletions.JSONRequest
}

func (model *fakeModel) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	model.requests = append(model.requests, request)
	return chatcompletions.Answer{Model: "local-27b", Object: json.RawMessage(`{"match":"possible","reason":" Close. ",
		"strengths":[{"point":"Scheduling","entries":["E2","[E99]"]}],"weaknesses":[{"point":"No Go","entries":[]}]}`)}, nil
}

type noRates struct{}

func (noRates) GetRates(context.Context, string) (map[string]float64, error) { return nil, nil }

func startHub(t *testing.T) *store.Store {
	t.Helper()
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveJobCriteria(ctx, owner, store.JobCriteria{Roles: []string{"Product Engineer"}, ExcludedRoleTerms: []string{"Sales"}, SeniorityLevels: []string{"Senior"}}); err != nil {
		t.Fatal(err)
	}
	role, err := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: "role", Title: "Senior Engineer", Organization: "Maple", StartMonth: "2024-01", Source: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: "case", RoleID: &role.ID, Title: "Built the scheduler", Outcome: "Cut no-shows", Source: "owner"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentPromptKindJobBrief, Body: "Brief it.\n{{job_criteria}}\n{{knowledge_base}}",
		ResultSchema: json.RawMessage(`{"type":"object"}`)}); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Senior Product Engineer", "Sales Manager"} {
		job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: title, URL: "https://acme.com/" + title, Description: "Build."})
		if err != nil {
			t.Fatal(err)
		}
		facts, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
		awaiting, _ := hub.GetJobForFacts(ctx, job.ID)
		if err := hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: job.ID, PromptID: facts.ID, Model: "m", TextHash: awaiting.TextHash, Facts: json.RawMessage(`{"technologies":["React"]}`)}); err != nil {
			t.Fatal(err)
		}
	}
	return hub
}

func TestAPassBriefsFittingJobsFromTheKnowledgeBaseAndSkipsPoorOnes(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	model := &fakeModel{}
	writer := jobbriefs.NewWriter(hub, model, noRates{})

	summary, err := writer.WriteOnce(ctx)
	if err != nil || summary.Written != 1 || summary.Skipped != 1 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	request := model.requests[0]
	if !strings.Contains(request.System, "[E1] Role: Senior Engineer at Maple (2024-01 – present)") ||
		!strings.Contains(request.System, "  [E2] Case: Built the scheduler. Outcome: Cut no-shows") ||
		!strings.Contains(request.System, "Roles: Product Engineer") || request.SchemaName != store.AgentPromptKindJobBrief {
		t.Errorf("system prompt =\n%s", request.System)
	}
	if !strings.Contains(request.User, `Facts already read from the posting: {"technologies":["React"]}`) || !strings.Contains(request.User, "Title: Senior Product Engineer") {
		t.Errorf("user message =\n%s", request.User)
	}

	items, _, _ := hub.ListJobs(ctx, store.JobFilter{Query: "senior product"})
	brief, err := hub.GetJobBrief(ctx, items[0].Job.ID)
	if err != nil || brief == nil || brief.Tier != store.JobBriefTierPre || brief.Model != "local-27b" || brief.Reason != "Close." {
		t.Fatalf("brief = %+v, %v", brief, err)
	}
	if len(brief.Strengths) != 1 || len(brief.Strengths[0].EntryIDs) != 1 || len(brief.CitedEntries) != 1 || brief.CitedEntries[0].Title != "Built the scheduler" {
		t.Errorf("strengths = %+v, cited = %+v; want E2 kept and E99 dropped", brief.Strengths, brief.CitedEntries)
	}

	if summary, _ := writer.WriteOnce(ctx); summary.Written != 0 {
		t.Errorf("a second pass wrote %d briefs; the brief is current", summary.Written)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: "skill", Title: "Go", Source: "owner"}); err != nil {
		t.Fatal(err)
	}
	if summary, _ := writer.WriteOnce(ctx); summary.Written != 1 {
		t.Errorf("after the knowledge base changed, a pass wrote %d briefs, want 1", summary.Written)
	}
}

func TestNoPromptMeansNoBriefs(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	model := &fakeModel{}
	if summary, err := jobbriefs.NewWriter(hub, model, noRates{}).WriteOnce(context.Background()); err != nil || summary != (jobbriefs.PassSummary{}) || len(model.requests) != 0 {
		t.Fatalf("summary = %+v, %v, %d requests", summary, err, len(model.requests))
	}
}

func TestGoodFitsAreBriefedBeforeUnclearOnes(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	// Good: the criteria's stack and a Brazil-friendly location make every
	// check a yes.
	if _, err := hub.SaveJobCriteria(ctx, owner, store.JobCriteria{Roles: []string{"Product Engineer"}, ExcludedRoleTerms: []string{"Sales"},
		SeniorityLevels: []string{"Senior"}, Technologies: []string{"React"}, HomeCountry: "Brazil", EligibleLocationTerms: []string{"LATAM"}}); err != nil {
		t.Fatal(err)
	}
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Senior Product Engineer, Growth", URL: "https://acme.com/growth", Location: "LATAM", Description: "Build."})
	facts, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	awaiting, _ := hub.GetJobForFacts(ctx, job.ID)
	if err := hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: job.ID, PromptID: facts.ID, Model: "m", TextHash: awaiting.TextHash,
		Facts: json.RawMessage(`{"technologies":["React"],"location_restriction":"LATAM","seniority":"Senior"}`)}); err != nil {
		t.Fatal(err)
	}
	// A newer job that stays unclear: newest first alone would brief it first.
	newer, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Senior Product Engineer, Platform", URL: "https://acme.com/platform", Description: "Build."})
	awaiting, _ = hub.GetJobForFacts(ctx, newer.ID)
	if err := hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: newer.ID, PromptID: facts.ID, Model: "m", TextHash: awaiting.TextHash,
		Facts: json.RawMessage(`{"technologies":["React"]}`)}); err != nil {
		t.Fatal(err)
	}
	model := &fakeModel{}
	if _, err := jobbriefs.NewWriter(hub, model, noRates{}).WriteOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 3 || !strings.Contains(model.requests[0].User, "Title: Senior Product Engineer, Growth") {
		t.Fatalf("first briefed:\n%s", model.requests[0].User)
	}
}
