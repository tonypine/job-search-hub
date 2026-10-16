package jobbriefs_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/claudeprint"
	"github.com/tonypine/job-search-hub/server/internal/jobbriefs"
	"github.com/tonypine/job-search-hub/server/internal/modelrouter"
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

// matchModel answers every brief with the match it holds.
type matchModel struct {
	match    string
	model    string
	requests int
	// kinds and byOwner are each request's run kind and whether the owner
	// asked for it.
	kinds   []string
	byOwner []bool
}

func (model *matchModel) CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	model.requests++
	model.kinds = append(model.kinds, request.GetRunKind())
	model.byOwner = append(model.byOwner, chatcompletions.IsStartedByOwner(ctx))
	return chatcompletions.Answer{Model: model.model, Object: json.RawMessage(`{"match":"` + model.match + `","reason":"R","strengths":[],"weaknesses":[]}`)}, nil
}

func TestFullBriefsGoToTheBestUndecidedJobsAndOnDemand(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobBrief)
	hash, _ := hub.GetKnowledgeHash(ctx)
	matches := map[string]string{"Strong one": "strong", "Stretch one": "stretch", "Strong on the pipeline": "strong"}
	ids := map[string]uuid.UUID{}
	for title, match := range matches {
		job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: title, URL: "https://acme.com/" + title})
		if err != nil {
			t.Fatal(err)
		}
		ids[title] = job.ID
		if err := hub.SaveJobBrief(ctx, store.JobBrief{JobID: job.ID, Tier: store.JobBriefTierPre, PromptID: prompt.ID, Model: "local", Match: match, Reason: "r", KnowledgeHash: hash}); err != nil {
			t.Fatal(err)
		}
	}
	onPipeline := ids["Strong on the pipeline"]
	if _, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &onPipeline}); err != nil {
		t.Fatal(err)
	}

	writer := jobbriefs.NewWriter(hub, &fakeModel{}, noRates{})
	if _, err := writer.WriteNightlyFullBriefs(ctx); !errors.Is(err, jobbriefs.ErrNoFullBriefs) {
		t.Fatalf("without Claude: %v", err)
	}
	claude := &matchModel{match: "possible", model: "claude-sonnet"}
	writer.FullClient = claude
	summary, err := writer.WriteNightlyFullBriefs(ctx)
	if err != nil || summary.Written != 1 {
		t.Fatalf("nightly = %+v, %v; want only the strong job off the pipeline", summary, err)
	}
	if brief, _ := hub.GetJobBrief(ctx, ids["Strong one"]); brief.Tier != store.JobBriefTierFull || brief.Model != "claude-sonnet" || brief.Match != "possible" {
		t.Errorf("strong job's brief = %+v", brief)
	}
	if summary, _ := writer.WriteNightlyFullBriefs(ctx); summary.Written != 0 {
		t.Errorf("a second night wrote %d, want 0", summary.Written)
	}

	if err := writer.WriteFullBrief(ctx, ids["Stretch one"]); err != nil {
		t.Fatal(err)
	}
	if brief, _ := hub.GetJobBrief(ctx, ids["Stretch one"]); brief.Tier != store.JobBriefTierFull {
		t.Errorf("on demand: %+v", brief)
	}
	if err := writer.WriteFullBrief(ctx, uuid.New()); !errors.Is(err, store.ErrJobNotFound) {
		t.Errorf("an unknown job: %v", err)
	}
	// Full briefs are runs of their own kind; the one asked for is the
	// owner's.
	if !slices.Equal(claude.kinds, []string{store.RunKindJobBriefFull, store.RunKindJobBriefFull}) || !slices.Equal(claude.byOwner, []bool{false, true}) {
		t.Errorf("full brief runs = %v, by the owner %v", claude.kinds, claude.byOwner)
	}
}

// startMacModel is a model server on this Mac that briefs every job a
// possible match, and counts its requests.
func startMacModel(t *testing.T) (string, *int) {
	t.Helper()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		answer, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop",
			"message": map[string]any{"content": `{"match":"possible","reason":"R","strengths":[],"weaknesses":[]}`}}}})
		w.Write(answer)
	}))
	t.Cleanup(server.Close)
	return server.URL, &requests
}

// writeFakeClaude is a Claude CLI that briefs every job a strong match, and
// writes a line to the file it returns for each run.
func writeFakeClaude(t *testing.T) (string, string) {
	t.Helper()
	folder := t.TempDir()
	runsFile := filepath.Join(folder, "runs")
	script := "#!/bin/sh\necho run >> " + runsFile + "\ncat <<'ANSWER'\n" +
		`{"is_error":false,"structured_output":{"match":"strong","reason":"R","strengths":[],"weaknesses":[]},"modelUsage":{"claude-sonnet-5-5":{}}}` +
		"\nANSWER\n"
	binary := filepath.Join(folder, "claude")
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, runsFile
}

func countClaudeRuns(t *testing.T, runsFile string) int {
	t.Helper()
	runs, err := os.ReadFile(runsFile)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(runs), "run")
}

func TestAFullBriefRunsOnTheModelItsRouteNames(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Staff Engineer", URL: "https://acme.com/staff", Description: "Build."})
	if err != nil {
		t.Fatal(err)
	}
	claude, err := hub.EnsureClaudeTaskRoutes(ctx, "sonnet", store.ClaudeTaskKinds)
	if err != nil {
		t.Fatal(err)
	}
	macURL, macRequests := startMacModel(t)
	mac, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "This Mac", BaseURL: macURL, EnforcesSchema: true})
	if err != nil {
		t.Fatal(err)
	}
	binary, claudeRuns := writeFakeClaude(t)
	router := modelrouter.New(hub)
	router.Claude = &claudeprint.Client{Binary: binary, Directory: t.TempDir()}
	writer := jobbriefs.NewWriter(hub, router, noRates{})
	writer.FullClient = router

	// Routed to this Mac, the full brief is written there, not on Claude.
	if _, err := hub.SaveTaskRoute(ctx, owner, store.RunKindJobBriefFull, store.TaskRouteInput{ProviderID: mac.ID, Model: "qwen3.8-27b"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFullBrief(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	brief, _ := hub.GetJobBrief(ctx, job.ID)
	if brief.Tier != store.JobBriefTierFull || brief.Model != "qwen3.8-27b" || brief.Match != "possible" || *macRequests != 1 || countClaudeRuns(t, claudeRuns) != 0 {
		t.Fatalf("on this Mac: brief %+v, %d requests to this Mac, %d Claude runs", brief, *macRequests, countClaudeRuns(t, claudeRuns))
	}

	// Routed back to Claude, the next one is Claude's.
	if _, err := hub.SaveTaskRoute(ctx, owner, store.RunKindJobBriefFull, store.TaskRouteInput{ProviderID: claude.ID, Model: "sonnet"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFullBrief(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	brief, _ = hub.GetJobBrief(ctx, job.ID)
	if brief.Model != "claude-sonnet-5-5" || brief.Match != "strong" || *macRequests != 1 || countClaudeRuns(t, claudeRuns) != 1 {
		t.Fatalf("on Claude: brief %+v, %d requests to this Mac, %d Claude runs", brief, *macRequests, countClaudeRuns(t, claudeRuns))
	}
}
