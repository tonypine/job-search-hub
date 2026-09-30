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
			kind = "company_triage"
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
	if status != http.StatusOK || string(answer) != `{"models":["Big-Q4.gguf","Small-Q4.gguf"]}`+"\n" {
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
