package modelrouter_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
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
	if len(records) != 2 || records[0].Outcome != chatcompletions.RunFailed || records[1].BaseURL != fallbackServer.url {
		t.Fatalf("records = %+v; want the failed attempt, then the fallback's", records)
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
	if _, err := router.CompleteJSON(context.Background(), request); !errors.Is(err, chatcompletions.ErrUnreachable) {
		t.Fatalf("a runtime that can't load: %v", err)
	}
	if len(records) != 1 || records[0].BaseURL != modelrouter.HubRuntimeAddress || records[0].Outcome != chatcompletions.RunFailed {
		t.Fatalf("records = %+v; want the failed start recorded", records)
	}
}
