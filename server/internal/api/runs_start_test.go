package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/cvdrafts"
	"github.com/tonypine/job-search-hub/server/internal/cvscreens"
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/hubevents"
	"github.com/tonypine/job-search-hub/server/internal/interviewpacks"
	"github.com/tonypine/job-search-hub/server/internal/jobbriefs"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/modelrouter"
	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

var ownerActor = store.Actor{Kind: store.ActorOwner}

type runStartAPI struct {
	url      string
	hub      *store.Store
	queue    *modelqueue.Queue
	controls *modelwork.Controls
}

// startRunStartAPI serves the run start routes over the hub's own workers,
// on a router that records each model call as the hub does, beside the
// routes the Mac runs agents through.
func startRunStartAPI(t *testing.T) runStartAPI {
	t.Helper()
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	pauses, err := hub.GetRunKindPauses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	queue := modelqueue.New(pauses, modelqueue.Settings{})
	router := modelrouter.New(hub)
	router.Queue = queue
	// A failure to record shows as a run missing from the list.
	router.RecordRun = func(ctx context.Context, record chatcompletions.RunRecord) {
		_, _ = hub.RecordTaskRun(ctx, store.NewTaskRun{
			ID: record.ID, Kind: record.Kind, SubjectID: record.Task.SubjectID, BaseURL: record.BaseURL, Provider: record.Provider, Model: record.Model,
			PromptID: record.Task.PromptID, PromptVersion: record.Task.PromptVersion, InputHash: record.InputHash, Output: record.Output,
			PromptTokens: record.PromptTokens, CompletionTokens: record.CompletionTokens, CostUSD: record.CostUSD, StartedAt: record.StartedAt,
			Duration: record.Duration, Outcome: record.Outcome, Error: record.Error, StartedByOwner: record.StartedByOwner,
		})
	}
	briefs := jobbriefs.NewWriter(hub, router, stubRates{})
	briefs.FullClient = router
	drainer := drain.New(time.Hour)
	recorder := hubevents.NewRecorder(hub, hubevents.NewBroadcaster())
	controls := &modelwork.Controls{
		Hub: hub, Queue: queue, Runtime: stubRuntime{}, Drain: drainer, Updates: recorder,
		Facts: jobfacts.NewExtractor(hub, router), Briefs: briefs, FullBriefs: briefs, CVs: cvdrafts.NewDrafter(hub, router),
		Screens: cvscreens.NewScreener(hub, router), Packs: interviewpacks.NewPreparer(hub, router),
	}
	routes := http.NewServeMux()
	api.RegisterRunStartRoutes(routes, controls, requireOwner)
	api.RegisterRunRoutes(routes, hub, requireOwner)
	api.RegisterRunKindRoutes(routes, controls, requireOwner)
	api.RegisterTaskRoutes(routes, hub, recorder, requireOwner)
	api.RegisterAgentRunRoutes(routes, hub, drainer, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return runStartAPI{url: server.URL, hub: hub, queue: queue, controls: controls}
}

// jobKinds are the kinds a job can run, in the catalog's order.
var jobKinds = []string{store.AgentPromptKindJobFacts, store.AgentPromptKindJobBrief, store.RunKindJobBriefFull, store.AgentPromptKindJobCV,
	store.AgentPromptKindRecruiterScreen, store.AgentPromptKindInterviewPrep, store.AgentRunKindJobFix}

// routeJobKinds routes every model kind of a job to model on a provider of
// its own, as the hub's settings would.
func routeJobKinds(t *testing.T, hub *store.Store, model string) store.ModelProvider {
	t.Helper()
	provider, err := hub.SaveModelProvider(context.Background(), ownerActor, nil, store.ModelProviderInput{Name: "This Mac", BaseURL: "http://127.0.0.1:1/v1", EnforcesSchema: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range jobKinds[:len(jobKinds)-1] {
		if _, err := hub.SaveTaskRoute(context.Background(), ownerActor, kind, store.TaskRouteInput{ProviderID: provider.ID, Model: model}); err != nil {
			t.Fatal(err)
		}
	}
	return provider
}

func addJob(t *testing.T, hub *store.Store, title string) store.Job {
	t.Helper()
	job, _, err := hub.AddManualJob(context.Background(), ownerActor, store.ManualJobInput{Title: title, URL: "https://acme.example/" + uuid.NewString(), Description: "Build the scheduler."})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func listRunnableKinds(t *testing.T, url string) []modelwork.RunnableKind {
	t.Helper()
	status, answer := send(t, http.MethodGet, url, ownerToken, "")
	var listed struct {
		Kinds []modelwork.RunnableKind `json:"kinds"`
	}
	if err := json.Unmarshal(answer, &listed); err != nil || status != http.StatusOK {
		t.Fatalf("GET %s: %d %s", url, status, answer)
	}
	return listed.Kinds
}

func startRun(t *testing.T, service runStartAPI, body string) (int, modelwork.StartedRun, string) {
	t.Helper()
	status, answer := send(t, http.MethodPost, service.url+"/v1/runs", ownerToken, body)
	var started struct {
		Run modelwork.StartedRun `json:"run"`
	}
	json.Unmarshal(answer, &started)
	return status, started.Run, string(answer)
}

// listSubjectRuns lists the subject's runs, the latest first.
func listSubjectRuns(t *testing.T, service runStartAPI, subjectID uuid.UUID) []store.Run {
	t.Helper()
	status, answer := send(t, http.MethodGet, service.url+"/v1/runs?subject_id="+subjectID.String(), ownerToken, "")
	var page store.RunPage
	if err := json.Unmarshal(answer, &page); err != nil || status != http.StatusOK {
		t.Fatalf("list runs: %d %s", status, answer)
	}
	return page.Runs
}

func TestEveryKindForAJobOrACompanyCanBeStarted(t *testing.T) {
	for _, kind := range store.RunKinds {
		if kind.SubjectType != store.RunSubjectJob && kind.SubjectType != store.RunSubjectCompany {
			continue
		}
		if !modelwork.IsStartable(kind.ID) {
			t.Errorf("%s runs for a %s but has no starter", kind.ID, kind.SubjectType)
		}
	}
}

func TestAJobListsEveryKindItCanRunWithItsModelAndLastRun(t *testing.T) {
	service := startRunStartAPI(t)
	ctx := context.Background()
	routeJobKinds(t, service.hub, "qwen3.8-27b")
	job := addJob(t, service.hub, "Staff Engineer")
	readAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if _, err := service.hub.RecordTaskRun(ctx, store.NewTaskRun{
		Kind: store.AgentPromptKindJobFacts, SubjectID: &job.ID, Provider: "This Mac", Model: "qwen3.8-27b", StartedAt: readAt, Outcome: store.RunOutcomeInvalid,
		Error: "the model's answer is invalid",
	}); err != nil {
		t.Fatal(err)
	}

	kinds := listRunnableKinds(t, service.url+"/v1/jobs/"+job.ID.String()+"/runnable-kinds")
	listed := []string{}
	byKind := map[string]modelwork.RunnableKind{}
	for _, kind := range kinds {
		listed, byKind[kind.ID] = append(listed, kind.ID), kind
	}
	if !slices.Equal(listed, jobKinds) {
		t.Fatalf("a job's kinds = %v, want %v", listed, jobKinds)
	}
	for _, id := range jobKinds[:len(jobKinds)-1] {
		if route := byKind[id].Route; route == nil || route.Model != "qwen3.8-27b" || route.Provider != "This Mac" {
			t.Errorf("%s's route = %+v", id, route)
		}
	}
	facts := byKind[store.AgentPromptKindJobFacts]
	if facts.LastRun == nil || !facts.LastRun.StartedAt.Equal(readAt) || facts.LastRun.Outcome != store.RunOutcomeInvalid || !facts.CanRun {
		t.Errorf("job facts = %+v, want its invalid run at %s", facts, readAt)
	}
	if byKind[store.RunKindJobBriefFull].LastRun != nil || !byKind[store.RunKindJobBriefFull].CanRun {
		t.Errorf("full briefs = %+v, want no run yet and ready", byKind[store.RunKindJobBriefFull])
	}
	for id, reason := range map[string]string{
		store.AgentPromptKindJobBrief:        "needs the job's facts first",
		store.AgentPromptKindJobCV:           "needs your base CV first",
		store.AgentPromptKindRecruiterScreen: "needs the tailored CV first",
		store.AgentPromptKindInterviewPrep:   "once you pursue the job",
	} {
		if byKind[id].CanRun || byKind[id].Reason != reason {
			t.Errorf("%s = can run %v, reason %q; want %q", id, byKind[id].CanRun, byKind[id].Reason, reason)
		}
	}
	if fix := byKind[store.AgentRunKindJobFix]; !fix.CanRun || !fix.NeedsNote || fix.Route != nil {
		t.Errorf("job fix = %+v", fix)
	}

	company, _, _ := service.hub.CreateCompany(ctx, ownerActor, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	companyKinds := listRunnableKinds(t, service.url+"/v1/companies/"+company.ID.String()+"/runnable-kinds")
	if len(companyKinds) != 2 || companyKinds[0].ID != store.AgentRunKindCompanyTriage || companyKinds[1].ID != store.AgentRunKindJobFinder || !companyKinds[0].CanRun {
		t.Errorf("a company's kinds = %+v", companyKinds)
	}
	for _, url := range []string{"/v1/jobs/" + uuid.NewString() + "/runnable-kinds", "/v1/companies/" + uuid.NewString() + "/runnable-kinds", "/v1/jobs/x/runnable-kinds"} {
		if status, _ := send(t, http.MethodGet, service.url+url, ownerToken, ""); status != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", url, status)
		}
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/jobs/"+job.ID.String()+"/runnable-kinds", "", ""); status != http.StatusUnauthorized {
		t.Errorf("without the owner token: %d", status)
	}
}

// startChatServer is a fake OpenRouter that tells received of each
// request's model and answers it once proceed closes, with a possible match.
func startChatServer(t *testing.T, received chan<- string, proceed <-chan struct{}) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		received <- fmt.Sprint(body["model"])
		<-proceed
		completion, _ := json.Marshal(map[string]any{
			"model": body["model"],
			"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{
				"content": `{"match":"possible","reason":"Close.","strengths":[],"weaknesses":[]}`}}},
			"usage": map[string]any{"prompt_tokens": 900, "completion_tokens": 120, "cost": 0.004},
		})
		w.Write(completion)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/api/v1"
}

func TestAPausedFullBriefStartedOnAnOpenRouterModelRunsAheadOfBackgroundWorkOnThatModel(t *testing.T) {
	service := startRunStartAPI(t)
	ctx := context.Background()
	routeJobKinds(t, service.hub, "qwen3.8-27b")
	if _, err := service.hub.SaveAgentPrompt(ctx, ownerActor, store.NewAgentPrompt{Kind: store.AgentPromptKindJobBrief, Body: "Brief it.",
		ResultSchema: json.RawMessage(`{"type":"object"}`)}); err != nil {
		t.Fatal(err)
	}
	job := addJob(t, service.hub, "Staff Engineer")
	received, proceed := make(chan string, 1), make(chan struct{})
	key := "sk-or-key"
	openRouter, err := service.hub.SaveModelProvider(ctx, ownerActor, nil, store.ModelProviderInput{
		Kind: store.ModelProviderKindOpenRouter, Name: "OpenRouter", BaseURL: startChatServer(t, received, proceed), APIKey: &key, EnforcesSchema: true})
	if err != nil {
		t.Fatal(err)
	}
	sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/resume-all")
	sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/job_brief_full/pause")

	// Background work holds the queue, and more waits behind it.
	releaseRunning := receiveTurn(t, waitForTurn(service.queue, store.AgentPromptKindJobFacts), "the running facts")
	waiting := waitForTurn(service.queue, store.AgentPromptKindJobBrief)

	status, started, answer := startRun(t, service, fmt.Sprintf(`{"kind":"job_brief_full","subject_id":%q,"provider_id":%q,"model":"anthropic/claude-sonnet-4.5"}`,
		job.ID, openRouter.ID))
	if status != http.StatusAccepted || started.ID == uuid.Nil || started.Provider != "OpenRouter" || started.Model != "anthropic/claude-sonnet-4.5" ||
		!started.ChosenModel || !started.StartedByOwner || started.SubjectType != store.RunSubjectJob || started.SubjectID != job.ID {
		t.Fatalf("start: %d %s", status, answer)
	}
	waitFor(t, "the brief to wait its turn", func() bool {
		queued := service.queue.Status().Waiting
		return len(queued) == 2 && queued[0].Kind == store.RunKindJobBriefFull && queued[0].Priority == "dispatched"
	})
	releaseRunning()
	select {
	case model := <-received:
		if model != "anthropic/claude-sonnet-4.5" {
			t.Fatalf("OpenRouter was asked for %s", model)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the started brief never reached OpenRouter")
	}
	select {
	case <-waiting:
		t.Fatal("background work went ahead of the started brief")
	case <-time.After(100 * time.Millisecond):
	}
	close(proceed)
	receiveTurn(t, waiting, "the waiting brief")()

	var run store.Run
	waitFor(t, "the brief's run", func() bool {
		runs := listSubjectRuns(t, service, job.ID)
		if len(runs) == 0 {
			return false
		}
		run = runs[0]
		return true
	})
	if run.ID != started.ID || run.Kind != store.RunKindJobBriefFull || run.Model != "anthropic/claude-sonnet-4.5" || run.Provider != "OpenRouter" ||
		!run.StartedByOwner || run.Outcome != store.RunOutcomeSucceeded {
		t.Fatalf("the job's run = %+v, want started %s", run, started.ID)
	}
	// The run is recorded when the model answers, before the brief is saved.
	var brief *store.JobBrief
	waitFor(t, "the job's brief", func() bool {
		brief, _ = service.hub.GetJobBrief(ctx, job.ID)
		return brief != nil
	})
	if brief.Tier != store.JobBriefTierFull || brief.Model != "anthropic/claude-sonnet-4.5" {
		t.Fatalf("the job's brief = %+v", brief)
	}
	if route, _ := service.hub.GetTaskRoute(ctx, store.RunKindJobBriefFull); route.Model != "qwen3.8-27b" {
		t.Fatalf("the kind's route changed: %+v", route)
	}
	if !service.controls.IsPaused(store.RunKindJobBriefFull) {
		t.Fatal("full briefs were resumed")
	}
	waitFor(t, "the update that the brief was written", func() bool {
		updates, _ := service.hub.ListUpdatesAfter(ctx, 0)
		return slices.ContainsFunc(updates, func(update store.Update) bool {
			return update.Kind == modelwork.RunFinishedUpdate && update.JobID != nil && *update.JobID == job.ID && strings.HasPrefix(update.Title, "Full brief written")
		})
	})
}

func TestAKindThatCantRunYetSaysWhyAndStartsNothing(t *testing.T) {
	service := startRunStartAPI(t)
	routeJobKinds(t, service.hub, "qwen3.8-27b")
	job := addJob(t, service.hub, "Staff Engineer")

	status, _, answer := startRun(t, service, fmt.Sprintf(`{"kind":"recruiter_screen","subject_id":%q}`, job.ID))
	if status != http.StatusConflict || !strings.Contains(answer, "needs the tailored CV first") {
		t.Fatalf("a screen with no tailored CV: %d %s", status, answer)
	}
	time.Sleep(100 * time.Millisecond)
	if runs := listSubjectRuns(t, service, job.ID); len(runs) != 0 || len(service.queue.Status().Waiting) != 0 {
		t.Fatalf("a refused start ran: runs %+v, queue %+v", runs, service.queue.Status())
	}

	company, _, _ := service.hub.CreateCompany(context.Background(), ownerActor, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	for body, want := range map[string]int{
		`{"kind":"mail_triage","subject_id":"` + job.ID.String() + `"}`:                                                    http.StatusBadRequest,
		`{"kind":"no_such_kind","subject_id":"` + job.ID.String() + `"}`:                                                   http.StatusBadRequest,
		`{"kind":"job_fix","subject_id":"` + job.ID.String() + `","note":"  "}`:                                            http.StatusBadRequest,
		`{"kind":"job_facts","subject_type":"company","subject_id":"` + job.ID.String() + `"}`:                             http.StatusBadRequest,
		`{"kind":"job_facts","subject_id":"` + job.ID.String() + `","provider_id":"` + uuid.NewString() + `"}`:             http.StatusBadRequest,
		`{"kind":"job_facts","subject_id":"` + job.ID.String() + `","provider_id":"` + uuid.NewString() + `","model":"m"}`: http.StatusBadRequest,
		`{"kind":"job_facts","subject_id":"` + uuid.NewString() + `"}`:                                                     http.StatusNotFound,
		`{"kind":"company_triage","subject_id":"` + job.ID.String() + `"}`:                                                 http.StatusNotFound,
		`{"kind":"job_finder","subject_id":"` + company.ID.String() + `","model":"opus"}`:                                  http.StatusBadRequest,
		`not json`: http.StatusBadRequest,
	} {
		if status, _, answer := startRun(t, service, body); status != want {
			t.Errorf("%s: %d %s, want %d", body, status, answer, want)
		}
	}

	// A kind the hub has no worker for is off.
	service.controls.Screens = nil
	kinds := listRunnableKinds(t, service.url+"/v1/jobs/"+job.ID.String()+"/runnable-kinds")
	if screen := kinds[slices.IndexFunc(kinds, func(kind modelwork.RunnableKind) bool { return kind.ID == store.AgentPromptKindRecruiterScreen })]; screen.CanRun ||
		screen.Reason != "no model is routed for it yet" {
		t.Errorf("an unrouted screen = %+v", screen)
	}
}

func TestCompanyResearchStartedOnClaudeIsListedAgainstTheCompanyOnceTheMacRunsIt(t *testing.T) {
	service := startRunStartAPI(t)
	ctx := context.Background()
	company, _, _ := service.hub.CreateCompany(ctx, ownerActor, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	claude, err := service.hub.SaveModelProvider(ctx, ownerActor, nil, store.ModelProviderInput{Kind: store.ModelProviderKindClaudeSubscription, Name: "Claude"})
	if err != nil {
		t.Fatal(err)
	}
	other := routeJobKinds(t, service.hub, "qwen3.8-27b")
	if status, _, answer := startRun(t, service, fmt.Sprintf(`{"kind":"company_triage","subject_id":%q,"provider_id":%q,"model":"m"}`, company.ID, other.ID)); status != http.StatusBadRequest ||
		!strings.Contains(answer, "only on Claude") {
		t.Fatalf("research on this Mac: %d %s", status, answer)
	}

	status, started, answer := startRun(t, service, fmt.Sprintf(`{"kind":"company_triage","subject_id":%q,"provider_id":%q,"model":"opus"}`, company.ID, claude.ID))
	if status != http.StatusAccepted || started.TaskID == nil || started.Model != "opus" || started.Provider != "Claude" || started.SubjectType != store.RunSubjectCompany {
		t.Fatalf("start: %d %s", status, answer)
	}

	// The Mac claims the task and runs the agent, as it does for the phone.
	queued, _ := service.hub.ListTasks(ctx, store.TaskQueued, 10)
	if len(queued) != 1 || queued[0].ID != *started.TaskID || queued[0].Kind != store.TaskResearchCompany || queued[0].Input != "acme.example" ||
		queued[0].CompanyID == nil || *queued[0].CompanyID != company.ID {
		t.Fatalf("the Mac's tasks = %+v", queued)
	}
	if status, answer := send(t, http.MethodPost, service.url+"/v1/tasks/"+started.TaskID.String()+"/claim", ownerToken, ""); status != http.StatusOK {
		t.Fatalf("claim: %d %s", status, answer)
	}
	status, body := send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"company_triage","input":"acme.example"}`)
	var agentRun startedRun
	if json.Unmarshal(body, &agentRun); status != http.StatusCreated || agentRun.AgentRun.ID != started.ID || agentRun.AgentRun.Model != "opus" {
		t.Fatalf("the Mac's run: %d %s, want id %s on opus", status, body, started.ID)
	}
	runs := listSubjectRuns(t, service, company.ID)
	if len(runs) != 1 || runs[0].ID != started.ID || runs[0].Kind != store.AgentRunKindCompanyTriage || !runs[0].StartedByOwner ||
		runs[0].Subject == nil || runs[0].Subject.Type != store.RunSubjectCompany || runs[0].Subject.Title != "Acme" {
		t.Fatalf("the company's runs = %+v", runs)
	}

	// A second run of the kind, which nobody started from the hub, gets an
	// id of its own and the kind's model.
	status, body = send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"company_triage","input":"acme.example"}`)
	var second startedRun
	if json.Unmarshal(body, &second); status != http.StatusCreated || second.AgentRun.ID == started.ID || second.AgentRun.Model != "" {
		t.Fatalf("a second run: %d %s", status, body)
	}
}

func TestReadingFactsTheFullBriefAndTheCVStartTheirKindsRun(t *testing.T) {
	service := startRunStartAPI(t)
	routeJobKinds(t, service.hub, "qwen3.8-27b")
	job := addJob(t, service.hub, "Staff Engineer")
	for path, kind := range map[string]string{"facts/read": store.AgentPromptKindJobFacts, "brief/full": store.RunKindJobBriefFull} {
		status, answer := send(t, http.MethodPost, service.url+"/v1/jobs/"+job.ID.String()+"/"+path, ownerToken, "")
		var started struct {
			Queued bool                 `json:"queued"`
			Run    modelwork.StartedRun `json:"run"`
		}
		if json.Unmarshal(answer, &started); status != http.StatusAccepted || !started.Queued || started.Run.Kind != kind || started.Run.Model != "qwen3.8-27b" {
			t.Errorf("POST %s: %d %s", path, status, answer)
		}
	}
	if status, answer := send(t, http.MethodPost, service.url+"/v1/jobs/"+job.ID.String()+"/cv", ownerToken, ""); status != http.StatusConflict ||
		!strings.Contains(string(answer), "needs your base CV first") {
		t.Errorf("POST cv with no base CV: %d %s", status, answer)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/jobs/"+uuid.NewString()+"/brief/full", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("an unknown job's brief: %d", status)
	}
	// Both runs fail, as this Mac's model isn't there, and say so.
	waitFor(t, "both runs to end", func() bool {
		updates, _ := service.hub.ListUpdatesAfter(context.Background(), 0)
		ended := 0
		for _, update := range updates {
			if update.Kind == modelwork.RunFinishedUpdate && strings.Contains(update.Title, "failed for Staff Engineer") {
				ended++
			}
		}
		return ended == 2
	})
}

// waitFor polls until done says so, for up to five seconds.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("gave up waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
