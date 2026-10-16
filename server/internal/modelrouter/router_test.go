package modelrouter_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/claudeprint"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/modelrouter"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// modelServer answers each request with the next of its answers, and keeps
// what it was asked.
type modelServer struct {
	url      string
	mutex    sync.Mutex
	answers  []string
	received []map[string]any
	headers  []http.Header
}

func startModelServer(t *testing.T, answers ...string) *modelServer {
	t.Helper()
	server := &modelServer{answers: answers}
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		server.received = append(server.received, body)
		server.headers = append(server.headers, r.Header.Clone())
		content := server.answers[0]
		if len(server.answers) > 1 {
			server.answers = server.answers[1:]
		}
		answer, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
		w.Write(answer)
	}))
	t.Cleanup(listener.Close)
	server.url = listener.URL
	return server
}

var request = chatcompletions.JSONRequest{
	System: "Record the facts.", User: "Title: Engineer", SchemaName: store.AgentPromptKindJobFacts, MaxTokens: 100,
	Schema: json.RawMessage(`{"type":"object","required":["stack"],"properties":{"stack":{"type":"string"}}}`),
}

func route(t *testing.T, hub *store.Store, providerURL string, enforces bool, key string, fallback *store.ModelProvider) store.ModelProvider {
	t.Helper()
	provider, err := hub.SaveModelProvider(context.Background(), owner, nil, store.ModelProviderInput{Name: providerURL, BaseURL: providerURL, APIKey: &key, EnforcesSchema: enforces})
	if err != nil {
		t.Fatal(err)
	}
	input := store.TaskRouteInput{ProviderID: provider.ID, Model: "routed-model"}
	if fallback != nil {
		input.FallbackProviderID, input.FallbackModel = &fallback.ID, "fallback-model"
	}
	if _, err := hub.SaveTaskRoute(context.Background(), owner, store.AgentPromptKindJobFacts, input); err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestARequestRunsOnTheRoutedProviderAndModel(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	server := startModelServer(t, `{"stack":"Go"}`)
	route(t, hub, server.url, true, "sk-test", nil)
	var records []chatcompletions.RunRecord
	router := modelrouter.New(hub)
	router.RecordRun = func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }

	answer, err := router.CompleteJSON(context.Background(), request)
	if err != nil || string(answer.Object) != `{"stack":"Go"}` || answer.Model != "routed-model" {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if server.received[0]["model"] != "routed-model" || server.headers[0].Get("Authorization") != "Bearer sk-test" {
		t.Fatalf("request = %v, headers = %v", server.received[0], server.headers[0])
	}
	if len(records) != 1 || records[0].BaseURL != server.url || records[0].Model != "routed-model" {
		t.Fatalf("records = %+v", records)
	}

	if _, err := router.CompleteJSON(context.Background(), chatcompletions.JSONRequest{SchemaName: store.AgentPromptKindMailTriage}); err == nil {
		t.Fatal("a kind with no route answered")
	}
}

func TestAnUnreachableProviderFallsBack(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	fallbackServer := startModelServer(t, `{"stack":"Rust"}`)
	fallback, _ := hub.SaveModelProvider(context.Background(), owner, nil, store.ModelProviderInput{Name: "Fallback", BaseURL: fallbackServer.url, EnforcesSchema: true})
	route(t, hub, "http://127.0.0.1:9", true, "", &fallback)
	var records []chatcompletions.RunRecord
	router := modelrouter.New(hub)
	router.RecordRun = func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }

	answer, err := router.CompleteJSON(context.Background(), request)
	if err != nil || answer.Model != "fallback-model" || string(answer.Object) != `{"stack":"Rust"}` {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if len(records) != 2 || records[0].Outcome != chatcompletions.RunFailed || records[0].FallbackFor != "" || records[1].BaseURL != fallbackServer.url ||
		records[1].FallbackFor != "http://127.0.0.1:9" {
		t.Fatalf("records = %+v; want the failed attempt, then the fallback's, standing in for the route's provider", records)
	}
}

func TestAProviderThatCannotEnforceSchemasGetsOneRetry(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	server := startModelServer(t, `{"stack":3}`, `{"stack":"Go"}`)
	route(t, hub, server.url, false, "", nil)

	answer, err := modelrouter.New(hub).CompleteJSON(context.Background(), request)
	if err != nil || string(answer.Object) != `{"stack":"Go"}` || len(server.received) != 2 {
		t.Fatalf("answer = %+v, %v after %d requests", answer, err, len(server.received))
	}
	if format := server.received[0]["response_format"].(map[string]any); format["type"] != "json_object" {
		t.Fatalf("response_format = %v", format)
	}

	stubborn := startModelServer(t, `{"stack":3}`)
	stubbornHub := store.New(testdatabase.New(t))
	route(t, stubbornHub, stubborn.url, false, "", nil)
	if _, err := modelrouter.New(stubbornHub).CompleteJSON(context.Background(), request); !errors.Is(err, chatcompletions.ErrInvalidAnswer) || len(stubborn.received) != 2 {
		t.Fatalf("err = %v after %d requests; want invalid after one retry", err, len(stubborn.received))
	}
}

// fakeRuntime serves every model from one test server and counts releases.
type fakeRuntime struct {
	url      string
	acquired []string
	released int
	failWith error
}

func (runtime *fakeRuntime) Acquire(_ context.Context, modelFile string) (string, func(), error) {
	if runtime.failWith != nil {
		return "", nil, runtime.failWith
	}
	runtime.acquired = append(runtime.acquired, modelFile)
	return runtime.url, func() { runtime.released++ }, nil
}

func TestAHubRuntimeRouteRunsOnTheHubsOwnRuntime(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	server := startModelServer(t, `{"stack":"Go"}`)
	provider, err := hub.SaveModelProvider(context.Background(), owner, nil, store.ModelProviderInput{Kind: store.ModelProviderKindHubRuntime, Name: "Hub runtime", EnforcesSchema: true})
	if err != nil || provider.BaseURL != "" {
		t.Fatalf("provider = %+v, %v", provider, err)
	}
	if _, err := hub.SaveTaskRoute(context.Background(), owner, store.AgentPromptKindJobFacts, store.TaskRouteInput{ProviderID: provider.ID, Model: "Qwen3.8-27B-Q4_K_M.gguf"}); err != nil {
		t.Fatal(err)
	}

	router := modelrouter.New(hub)
	if _, err := router.CompleteJSON(context.Background(), request); !errors.Is(err, chatcompletions.ErrUnreachable) {
		t.Fatalf("without a runtime: %v", err)
	}
	runtime := &fakeRuntime{url: server.url + "/v1"}
	router.Runtime = runtime
	answer, err := router.CompleteJSON(context.Background(), request)
	if err != nil || answer.Model != "Qwen3.8-27B-Q4_K_M.gguf" || len(runtime.acquired) != 1 || runtime.released != 1 {
		t.Fatalf("answer = %+v, %v, runtime = %+v", answer, err, runtime)
	}

	var records []chatcompletions.RunRecord
	router.RecordRun = func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }
	runtime.failWith = errors.New("llama-server exited while loading")
	if _, err := router.CompleteJSON(chatcompletions.WithStartedByOwner(context.Background()), request); !errors.Is(err, chatcompletions.ErrUnreachable) {
		t.Fatalf("a runtime that can't load: %v", err)
	}
	if len(records) != 1 || records[0].BaseURL != modelrouter.HubRuntimeAddress || records[0].Outcome != chatcompletions.RunFailed ||
		records[0].Provider != "Hub runtime" || !records[0].StartedByOwner {
		t.Fatalf("records = %+v; want the failed start recorded", records)
	}
	runtime.failWith = nil
	if _, err := router.CompleteJSON(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[1].Provider != "Hub runtime" || records[1].Outcome != chatcompletions.RunSucceeded || records[1].StartedByOwner {
		t.Errorf("records = %+v; want the run on the runtime recorded under its provider", records)
	}
}

// recordingQueue grants every ticket at once and keeps them.
type recordingQueue struct{ tickets []modelqueue.Ticket }

func (queue *recordingQueue) Wait(_ context.Context, ticket modelqueue.Ticket) (func(), error) {
	queue.tickets = append(queue.tickets, ticket)
	return func() {}, nil
}

func TestEachRequestWaitsItsTurnWithItsKindsPriority(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	server := startModelServer(t, `{"stack":"Go"}`)
	provider := route(t, hub, server.url, true, "", nil)
	if _, err := hub.SaveTaskRoute(context.Background(), owner, store.AgentPromptKindMailTriage, store.TaskRouteInput{ProviderID: provider.ID, Model: "small"}); err != nil {
		t.Fatal(err)
	}
	queue := &recordingQueue{}
	router := modelrouter.New(hub)
	router.Queue = queue

	router.CompleteJSON(context.Background(), request)
	mail := request
	mail.SchemaName = store.AgentPromptKindMailTriage
	router.CompleteJSON(context.Background(), mail)
	router.CompleteJSON(modelqueue.WithPriority(context.Background(), modelqueue.PriorityDispatched), request)

	want := []modelqueue.Ticket{
		{Kind: store.AgentPromptKindJobFacts, Model: "routed-model", Priority: modelqueue.PriorityBackground},
		{Kind: store.AgentPromptKindMailTriage, Model: "small", Priority: modelqueue.PrioritySorting},
		{Kind: store.AgentPromptKindJobFacts, Model: "routed-model", Priority: modelqueue.PriorityDispatched},
	}
	if fmt.Sprint(queue.tickets) != fmt.Sprint(want) {
		t.Fatalf("tickets = %+v\nwant      %+v", queue.tickets, want)
	}
}

func TestARequestOnAGivenModelSkipsTheRoute(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	routed := startModelServer(t, `{"stack":"Routed"}`)
	route(t, hub, routed.url, true, "", nil)
	other := startModelServer(t, `{"stack":"Go"}`)
	provider, err := hub.SaveModelProvider(context.Background(), owner, nil, store.ModelProviderInput{Name: "Other", BaseURL: other.url, EnforcesSchema: true})
	if err != nil {
		t.Fatal(err)
	}

	answer, err := modelrouter.New(hub).CompleteJSONOn(context.Background(), provider.ID, "chosen-model", request)
	if err != nil || string(answer.Object) != `{"stack":"Go"}` {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if len(routed.received) != 0 || other.received[0]["model"] != "chosen-model" {
		t.Fatalf("routed got %d requests; other got %v", len(routed.received), other.received)
	}
}

// startOpenRouter is a fake OpenRouter: it answers chat completions asked
// with key, the hub's attribution and a request for the cost, with answer
// and its usage, or with status and OpenRouter's error when status isn't 200.
func startOpenRouter(t *testing.T, key string, status int, answer string) (string, *[]map[string]any) {
	t.Helper()
	var received []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		received = append(received, body)
		usage, _ := body["usage"].(map[string]any)
		if r.URL.Path != "/api/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("X-Title") != "Job Search Hub" ||
			r.Header.Get("HTTP-Referer") == "" || usage["include"] != true {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"code":400,"message":"not asked as OpenRouter is"}}`))
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			fmt.Fprintf(w, `{"error":{"code":%d,"message":"No instances available","metadata":{"provider_name":"Anthropic"}}}`, status)
			return
		}
		completion, _ := json.Marshal(map[string]any{
			"model":   body["model"],
			"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": answer}}},
			"usage":   map[string]any{"prompt_tokens": 1200, "completion_tokens": 300, "total_tokens": 1500, "cost": 0.0081},
		})
		w.Write(completion)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/api/v1", &received
}

var screenRequest = chatcompletions.JSONRequest{
	System: "Screen the CV as its recruiter would.", User: "CV: Engineer", SchemaName: store.AgentPromptKindRecruiterScreen, MaxTokens: 100,
	Schema: json.RawMessage(`{"type":"object","required":["verdict"],"properties":{"verdict":{"type":"string"}}}`),
}

func addOpenRouter(t *testing.T, hub *store.Store, baseURL, key string) store.ModelProvider {
	t.Helper()
	provider, err := hub.SaveModelProvider(context.Background(), owner, nil, store.ModelProviderInput{
		Kind: store.ModelProviderKindOpenRouter, Name: "OpenRouter", BaseURL: baseURL, APIKey: &key, EnforcesSchema: true})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestARecruiterScreenRoutedToOpenRouterRecordsItsModelTokensAndCost(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	baseURL, received := startOpenRouter(t, "sk-or-test", http.StatusOK, `{"verdict":"likely interview"}`)
	provider := addOpenRouter(t, hub, baseURL, "sk-or-test")
	if _, err := hub.SaveTaskRoute(context.Background(), owner, store.AgentPromptKindRecruiterScreen,
		store.TaskRouteInput{ProviderID: provider.ID, Model: "anthropic/claude-sonnet-4.5"}); err != nil {
		t.Fatal(err)
	}
	var records []chatcompletions.RunRecord
	router := modelrouter.New(hub)
	router.RecordRun = func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }

	answer, err := router.CompleteJSON(context.Background(), screenRequest)
	if err != nil || string(answer.Object) != `{"verdict":"likely interview"}` || answer.Model != "anthropic/claude-sonnet-4.5" {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if len(*received) != 1 || (*received)[0]["model"] != "anthropic/claude-sonnet-4.5" {
		t.Fatalf("OpenRouter got %v", *received)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	record := records[0]
	if record.Kind != store.AgentPromptKindRecruiterScreen || record.Provider != "OpenRouter" || record.Model != "anthropic/claude-sonnet-4.5" ||
		record.PromptTokens != 1200 || record.CompletionTokens != 300 || record.CostUSD != "0.0081" || record.Outcome != chatcompletions.RunSucceeded {
		t.Fatalf("record = %+v", record)
	}
}

func TestWhenOpenRouterIsDownTheFallbackRunsAndItsRunSaysSo(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	baseURL, _ := startOpenRouter(t, "sk-or-test", http.StatusServiceUnavailable, "")
	provider := addOpenRouter(t, hub, baseURL, "sk-or-test")
	local := startModelServer(t, `{"verdict":"likely interview"}`)
	fallback, err := hub.SaveModelProvider(context.Background(), owner, nil, store.ModelProviderInput{Name: "This Mac", BaseURL: local.url, EnforcesSchema: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTaskRoute(context.Background(), owner, store.AgentPromptKindRecruiterScreen, store.TaskRouteInput{
		ProviderID: provider.ID, Model: "anthropic/claude-sonnet-4.5", FallbackProviderID: &fallback.ID, FallbackModel: "qwen"}); err != nil {
		t.Fatal(err)
	}
	var records []chatcompletions.RunRecord
	router := modelrouter.New(hub)
	router.RecordRun = func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }

	answer, err := router.CompleteJSON(context.Background(), screenRequest)
	if err != nil || answer.Model != "qwen" || string(answer.Object) != `{"verdict":"likely interview"}` {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if len(records) != 2 || records[0].Provider != "OpenRouter" || records[0].Outcome != chatcompletions.RunFailed ||
		!strings.Contains(records[0].Error, "OpenRouter is down or busy (503): No instances available (Anthropic)") {
		t.Fatalf("records = %+v; want OpenRouter's failure first", records)
	}
	if records[1].Provider != "This Mac" || records[1].Model != "qwen" || records[1].FallbackFor != "OpenRouter" || records[1].Outcome != chatcompletions.RunSucceeded {
		t.Fatalf("the fallback's record = %+v", records[1])
	}

	// A key OpenRouter refuses isn't OpenRouter down: no fallback runs.
	refusing, _ := startOpenRouter(t, "sk-or-test", http.StatusUnauthorized, "")
	if _, err := hub.SaveModelProvider(context.Background(), owner, &provider.ID, store.ModelProviderInput{
		Kind: store.ModelProviderKindOpenRouter, Name: "OpenRouter", BaseURL: refusing, EnforcesSchema: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := router.CompleteJSON(context.Background(), screenRequest); err == nil || errors.Is(err, chatcompletions.ErrUnreachable) ||
		!strings.Contains(err.Error(), "OpenRouter refused the key") || len(local.received) != 1 {
		t.Fatalf("a refused key: %v, the fallback got %d requests", err, len(local.received))
	}
}

// writeFakeClaude is a Claude CLI that answers answer.
func writeFakeClaude(t *testing.T, answer string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncat <<'ANSWER'\n"+answer+"\nANSWER\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary
}

var fullBriefRequest = chatcompletions.JSONRequest{
	System: "Brief the job.", User: "Title: Engineer", SchemaName: store.AgentPromptKindJobBrief, MaxTokens: 100,
	Schema: json.RawMessage(`{"type":"object","required":["match"],"properties":{"match":{"type":"string"}}}`),
	Task:   chatcompletions.TaskLabel{Kind: store.RunKindJobBriefFull},
}

func TestAClaudeRouteRunsOnTheCLIAndFallsBackToThisMacWithoutIt(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	claude, err := hub.EnsureClaudeTaskRoutes(ctx, "sonnet", store.ClaudeTaskKinds)
	if err != nil {
		t.Fatal(err)
	}
	var records []chatcompletions.RunRecord
	queue := &recordingQueue{}
	router := modelrouter.New(hub)
	router.Queue = queue
	router.RecordRun = func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }
	router.Claude = &claudeprint.Client{Binary: writeFakeClaude(t, `{"is_error":false,"structured_output":{"match":"strong"},"total_cost_usd":0.042,
		"usage":{"input_tokens":10,"output_tokens":120},"modelUsage":{"claude-sonnet-5-5":{}}}`), Directory: t.TempDir()}

	// A full brief takes its own kind's route, not the job_brief prompt's.
	answer, err := router.CompleteJSON(ctx, fullBriefRequest)
	if err != nil || string(answer.Object) != `{"match":"strong"}` || answer.Model != "claude-sonnet-5-5" {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if len(records) != 1 || records[0].Kind != store.RunKindJobBriefFull || records[0].Provider != claude.Name || records[0].BaseURL != claudeprint.BaseURL ||
		records[0].CostUSD != "0.042" {
		t.Fatalf("records = %+v", records)
	}
	if len(queue.tickets) != 0 {
		t.Fatalf("a Claude run waited in this Mac's queue: %+v", queue.tickets)
	}

	// Without the CLI, Claude is unreachable, and the fallback on this Mac
	// writes the brief.
	local := startModelServer(t, `{"match":"possible"}`)
	mac, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "This Mac", BaseURL: local.url, EnforcesSchema: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTaskRoute(ctx, owner, store.RunKindJobBriefFull, store.TaskRouteInput{ProviderID: claude.ID, Model: "sonnet",
		FallbackProviderID: &mac.ID, FallbackModel: "qwen"}); err != nil {
		t.Fatal(err)
	}
	router.Claude = nil
	answer, err = router.CompleteJSON(ctx, fullBriefRequest)
	if err != nil || answer.Model != "qwen" || string(answer.Object) != `{"match":"possible"}` {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if len(records) != 2 || records[1].Provider != "This Mac" || records[1].FallbackFor != claude.Name {
		t.Fatalf("records = %+v", records)
	}
	if len(queue.tickets) != 1 || queue.tickets[0].Kind != store.RunKindJobBriefFull || queue.tickets[0].Model != "qwen" {
		t.Fatalf("the fallback's turn = %+v", queue.tickets)
	}
}
