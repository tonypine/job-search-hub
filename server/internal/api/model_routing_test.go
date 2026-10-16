package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestAProviderKeyIsSavedButNeverServedBack(t *testing.T) {
	service := startAPI(t)
	status, answer := send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken,
		`{"name":"OpenRouter","base_url":"https://openrouter.ai/api/v1/","api_key":"sk-secret","enforces_schema":false}`)
	var provider store.ModelProvider
	if json.Unmarshal(answer, &provider); status != http.StatusCreated || !provider.HasKey || provider.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("created: %d %s", status, answer)
	}
	if strings.Contains(string(answer), "sk-secret") {
		t.Fatalf("the key came back: %s", answer)
	}
	_, answer = send(t, http.MethodGet, service.url+"/v1/model-providers", ownerToken, "")
	if strings.Contains(string(answer), "sk-secret") || !strings.Contains(string(answer), `"has_key":true`) {
		t.Fatalf("listed: %s", answer)
	}

	// Saving without a key keeps it; an empty key removes it.
	status, answer = send(t, http.MethodPut, service.url+"/v1/model-providers/"+provider.ID.String(), ownerToken,
		`{"name":"OpenRouter","base_url":"https://openrouter.ai/api/v1","enforces_schema":true}`)
	if json.Unmarshal(answer, &provider); status != http.StatusOK || !provider.HasKey || !provider.EnforcesSchema {
		t.Fatalf("kept the key: %d %s", status, answer)
	}
	_, answer = send(t, http.MethodPut, service.url+"/v1/model-providers/"+provider.ID.String(), ownerToken,
		`{"name":"OpenRouter","base_url":"https://openrouter.ai/api/v1","api_key":"","enforces_schema":true}`)
	if json.Unmarshal(answer, &provider); provider.HasKey {
		t.Fatalf("removed the key: %s", answer)
	}
}

func TestATaskIsRoutedToAProviderThatThenCannotBeDeleted(t *testing.T) {
	service := startAPI(t)
	_, answer := send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken, `{"name":"Local","base_url":"http://localhost:1234/v1","enforces_schema":true}`)
	var provider store.ModelProvider
	json.Unmarshal(answer, &provider)

	status, answer := send(t, http.MethodPut, service.url+"/v1/task-routes/job_facts", ownerToken, fmt.Sprintf(`{"provider_id":%q,"model":"qwen3.8-27b"}`, provider.ID))
	if status != http.StatusOK {
		t.Fatalf("route: %d %s", status, answer)
	}
	status, answer = send(t, http.MethodGet, service.url+"/v1/task-routes", ownerToken, "")
	var listed struct {
		Routes []store.TaskRoute `json:"routes"`
	}
	if json.Unmarshal(answer, &listed); status != http.StatusOK || len(listed.Routes) != 1 || listed.Routes[0].Model != "qwen3.8-27b" {
		t.Fatalf("routes: %d %s", status, answer)
	}
	if status, answer = send(t, http.MethodDelete, service.url+"/v1/model-providers/"+provider.ID.String(), ownerToken, ""); status != http.StatusConflict {
		t.Fatalf("deleting a provider in use: %d %s", status, answer)
	}

	for name, body := range map[string]string{
		"an unknown task":       fmt.Sprintf(`{"provider_id":%q,"model":"m"}`, provider.ID),
		"a half fallback":       fmt.Sprintf(`{"provider_id":%q,"model":"m","fallback_model":"other"}`, provider.ID),
		"an unknown provider":   `{"provider_id":"00000000-0000-0000-0000-000000000001","model":"m"}`,
		"a route with no model": fmt.Sprintf(`{"provider_id":%q,"model":" "}`, provider.ID),
	} {
		kind := "job_facts"
		if name == "an unknown task" {
			kind = "no_such_kind"
		}
		if status, answer := send(t, http.MethodPut, service.url+"/v1/task-routes/"+kind, ownerToken, body); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, answer)
		}
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/task-routes", "", ""); status != http.StatusUnauthorized {
		t.Errorf("without the owner token: %d", status)
	}
}

// makeModelsDir is a models folder with two GGUF files and one other file.
func makeModelsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"Small-Q4.gguf", "Big-Q4.gguf", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAProvidersModelsAreListed(t *testing.T) {
	service := startAPI(t)
	_, answer := send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken, `{"kind":"hub_runtime","name":"Hub runtime","enforces_schema":true}`)
	var runtime store.ModelProvider
	json.Unmarshal(answer, &runtime)
	status, answer := send(t, http.MethodGet, service.url+"/v1/model-providers/"+runtime.ID.String()+"/models", ownerToken, "")
	if status != http.StatusOK || string(answer) != `{"models":["Big-Q4.gguf","Small-Q4.gguf"],"catalog":[{"id":"Big-Q4.gguf"},{"id":"Small-Q4.gguf"}]}`+"\n" {
		t.Fatalf("runtime models: %d %s", status, answer)
	}

	served := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer sk-list" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"data":[{"id":"qwen/qwen3.5-9b"},{"id":"gemma"}]}`))
	}))
	defer served.Close()
	_, answer = send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken, fmt.Sprintf(`{"name":"Served","base_url":%q,"api_key":"sk-list","enforces_schema":true}`, served.URL+"/v1"))
	var hosted store.ModelProvider
	json.Unmarshal(answer, &hosted)
	status, answer = send(t, http.MethodGet, service.url+"/v1/model-providers/"+hosted.ID.String()+"/models", ownerToken, "")
	if status != http.StatusOK || !strings.Contains(string(answer), `["gemma","qwen/qwen3.5-9b"]`) {
		t.Fatalf("served models: %d %s", status, answer)
	}

	_, answer = send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken, `{"name":"Down","base_url":"http://127.0.0.1:9/v1","enforces_schema":true}`)
	var down store.ModelProvider
	json.Unmarshal(answer, &down)
	if status, _ := send(t, http.MethodGet, service.url+"/v1/model-providers/"+down.ID.String()+"/models", ownerToken, ""); status != http.StatusBadGateway {
		t.Errorf("a provider that doesn't answer: %d", status)
	}
}

// startOpenRouter is a fake OpenRouter catalogue, which serves only requests
// with key and the hub's attribution.
func startOpenRouter(t *testing.T, key string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" || r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("X-Title") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"code":401,"message":"No auth credentials found"}}`))
			return
		}
		w.Write([]byte(`{"data":[
			{"id":"openrouter/auto","name":"Auto Router","context_length":2000000,"pricing":{"prompt":"-1","completion":"-1"}},
			{"id":"anthropic/claude-sonnet-4.5","name":"Anthropic: Claude Sonnet 4.5","context_length":1000000,"pricing":{"prompt":"0.000003","completion":"0.000015"}},
			{"id":"qwen/qwen3-coder:free","name":"Qwen3 Coder (free)","context_length":262000,"pricing":{"prompt":"0","completion":"0"}}]}`))
	}))
	t.Cleanup(server.Close)
	return server.URL + "/api/v1"
}

func TestOpenRouterIsAddedWithAKeyAndItsModelsComeWithTheirPrices(t *testing.T) {
	service := startAPI(t)
	if status, answer := send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken, `{"kind":"openrouter","name":"OpenRouter"}`); status != http.StatusBadRequest ||
		!strings.Contains(string(answer), "OpenRouter needs a key") {
		t.Fatalf("OpenRouter with no key: %d %s", status, answer)
	}
	status, answer := send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken, `{"kind":"openrouter","name":"Preset","api_key":"sk-or-preset"}`)
	var preset store.ModelProvider
	if json.Unmarshal(answer, &preset); status != http.StatusCreated || preset.BaseURL != "https://openrouter.ai/api/v1" || preset.Type != store.ModelProviderTypeOpenAICompatible {
		t.Fatalf("the preset: %d %s", status, answer)
	}

	status, answer = send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken,
		fmt.Sprintf(`{"kind":"openrouter","name":"OpenRouter","base_url":%q,"api_key":"sk-or-secret"}`, startOpenRouter(t, "sk-or-secret")))
	var provider store.ModelProvider
	if json.Unmarshal(answer, &provider); status != http.StatusCreated || !provider.HasKey || provider.Kind != store.ModelProviderKindOpenRouter {
		t.Fatalf("created: %d %s", status, answer)
	}
	_, listed := send(t, http.MethodGet, service.url+"/v1/model-providers", ownerToken, "")
	if strings.Contains(string(answer)+string(listed), "sk-or-secret") {
		t.Fatalf("the key came back: %s %s", answer, listed)
	}

	status, answer = send(t, http.MethodGet, service.url+"/v1/model-providers/"+provider.ID.String()+"/models", ownerToken, "")
	var models struct {
		Models  []string `json:"models"`
		Catalog []struct {
			ID                  string  `json:"id"`
			Name                string  `json:"name"`
			InputUSDPerMillion  *string `json:"input_usd_per_million"`
			OutputUSDPerMillion *string `json:"output_usd_per_million"`
			ContextLength       int     `json:"context_length"`
		} `json:"catalog"`
	}
	if json.Unmarshal(answer, &models); status != http.StatusOK || len(models.Catalog) != 3 ||
		strings.Join(models.Models, ",") != "anthropic/claude-sonnet-4.5,openrouter/auto,qwen/qwen3-coder:free" {
		t.Fatalf("models: %d %s", status, answer)
	}
	sonnet, auto, free := models.Catalog[0], models.Catalog[1], models.Catalog[2]
	if sonnet.Name != "Anthropic: Claude Sonnet 4.5" || sonnet.InputUSDPerMillion == nil || *sonnet.InputUSDPerMillion != "3" ||
		sonnet.OutputUSDPerMillion == nil || *sonnet.OutputUSDPerMillion != "15" || sonnet.ContextLength != 1000000 {
		t.Errorf("a priced model: %+v", sonnet)
	}
	if auto.InputUSDPerMillion != nil || auto.OutputUSDPerMillion != nil {
		t.Errorf("a routed model has no price of its own: %+v", auto)
	}
	if free.InputUSDPerMillion == nil || *free.InputUSDPerMillion != "0" {
		t.Errorf("a free model: %+v", free)
	}

	// A key OpenRouter refuses says so.
	_, answer = send(t, http.MethodPut, service.url+"/v1/model-providers/"+provider.ID.String(), ownerToken,
		fmt.Sprintf(`{"kind":"openrouter","name":"OpenRouter","base_url":%q,"api_key":"sk-or-wrong"}`, provider.BaseURL))
	if status, answer = send(t, http.MethodGet, service.url+"/v1/model-providers/"+provider.ID.String()+"/models", ownerToken, ""); status != http.StatusBadGateway ||
		!strings.Contains(string(answer), "OpenRouter refused the key") {
		t.Errorf("a refused key: %d %s", status, answer)
	}
}

func TestAnAgentKindIsRoutedOnlyToClaude(t *testing.T) {
	service := startAPI(t)
	_, answer := send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken, `{"kind":"openrouter","name":"OpenRouter","api_key":"sk-or"}`)
	var openRouter store.ModelProvider
	json.Unmarshal(answer, &openRouter)
	status, answer := send(t, http.MethodPost, service.url+"/v1/model-providers", ownerToken, `{"kind":"claude_subscription","name":"Claude","base_url":"http://ignored"}`)
	var claude store.ModelProvider
	if json.Unmarshal(answer, &claude); status != http.StatusCreated || claude.Type != store.ModelProviderTypeClaudeSubscription || claude.BaseURL != "" {
		t.Fatalf("the Claude provider: %d %s", status, answer)
	}
	status, answer = send(t, http.MethodGet, service.url+"/v1/model-providers/"+claude.ID.String()+"/models", ownerToken, "")
	if status != http.StatusOK || !strings.Contains(string(answer), `"models":["haiku","opus","sonnet"]`) {
		t.Fatalf("Claude's models: %d %s", status, answer)
	}

	status, answer = send(t, http.MethodPut, service.url+"/v1/task-routes/company_triage", ownerToken,
		fmt.Sprintf(`{"provider_id":%q,"model":"anthropic/claude-sonnet-4.5"}`, openRouter.ID))
	if status != http.StatusBadRequest || !strings.Contains(string(answer), "agents run on Claude") {
		t.Fatalf("company research on OpenRouter: %d %s", status, answer)
	}
	status, answer = send(t, http.MethodPut, service.url+"/v1/task-routes/company_triage", ownerToken,
		fmt.Sprintf(`{"provider_id":%q,"model":"opus","fallback_provider_id":%q,"fallback_model":"sonnet"}`, claude.ID, claude.ID))
	if status != http.StatusBadRequest || !strings.Contains(string(answer), "no fallback") {
		t.Fatalf("an agent with a fallback: %d %s", status, answer)
	}
	if status, answer = send(t, http.MethodPut, service.url+"/v1/task-routes/company_triage", ownerToken,
		fmt.Sprintf(`{"provider_id":%q,"model":"opus"}`, claude.ID)); status != http.StatusOK {
		t.Fatalf("company research on Claude: %d %s", status, answer)
	}

	// The next run starts Claude Code on the route's model, unless the runner
	// names another.
	status, answer = send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"company_triage","input":"stripe.com"}`)
	var started startedRun
	if json.Unmarshal(answer, &started); status != http.StatusCreated || started.AgentRun.Model != "opus" {
		t.Fatalf("started on the route: %d %s", status, answer)
	}
	_, answer = send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"company_triage","input":"clio.com","model":"haiku"}`)
	if json.Unmarshal(answer, &started); started.AgentRun.Model != "haiku" {
		t.Fatalf("started on the runner's model: %s", answer)
	}

	// Full briefs, a one-shot Claude kind, run on any provider.
	if status, answer = send(t, http.MethodPut, service.url+"/v1/task-routes/job_brief_full", ownerToken,
		fmt.Sprintf(`{"provider_id":%q,"model":"anthropic/claude-sonnet-4.5","fallback_provider_id":%q,"fallback_model":"sonnet"}`, openRouter.ID, claude.ID)); status != http.StatusOK {
		t.Fatalf("full briefs on OpenRouter: %d %s", status, answer)
	}
}
