package modelruntime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/modelruntime"
)

type benchJob struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Location       string   `json:"location"`
	OtherLocations []string `json:"other_locations"`
	WorkplaceType  string   `json:"workplace_type"`
	EmploymentType string   `json:"employment_type"`
	Description    string   `json:"description"`
	Split          string   `json:"split"`
}

// formatBenchJob is the text the hub's job facts extractor sends.
func formatBenchJob(job benchJob) string {
	text := fmt.Sprintf("Title: %s\n", job.Title)
	if job.Location != "" {
		text += fmt.Sprintf("Location: %s\n", job.Location)
	}
	if len(job.OtherLocations) > 0 {
		text += fmt.Sprintf("Other locations: %s\n", strings.Join(job.OtherLocations, "; "))
	}
	if job.WorkplaceType != "" {
		text += fmt.Sprintf("Workplace: %s\n", job.WorkplaceType)
	}
	if job.EmploymentType != "" {
		text += fmt.Sprintf("Employment type: %s\n", job.EmploymentType)
	}
	description := job.Description
	if len(description) > 40_000 {
		description = description[:40_000]
	}
	return text + "\nDescription:\n" + description
}

func readJSONFile(t *testing.T, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// TestLiveRuntimeGivesTheBenchmarksAnswers runs the real llama-server on the
// real models and checks the answers match what the 2026-09-30 benchmark
// recorded for the same postings and prompt, then that a swap unloads the
// first model and an idle runtime stops. It needs HUB_LIVE_TEST=1,
// HUB_BENCH_DIR (the benchmark's folder) and HUB_MODELS_DIR.
func TestLiveRuntimeGivesTheBenchmarksAnswers(t *testing.T) {
	benchDir, modelsDir := os.Getenv("HUB_BENCH_DIR"), os.Getenv("HUB_MODELS_DIR")
	if os.Getenv("HUB_LIVE_TEST") != "1" || benchDir == "" || modelsDir == "" {
		t.Skip("set HUB_LIVE_TEST=1, HUB_BENCH_DIR and HUB_MODELS_DIR to run the real models")
	}
	llamaServer, err := exec.LookPath("llama-server")
	if err != nil {
		t.Skip("llama-server isn't on PATH")
	}
	// The benchmark's P0s prompt is today's job_facts prompt with its schema
	// appended as the hub appends it; the schema text is split back out so
	// the client appends the very same bytes.
	var benchmarkPrompt struct {
		System string `json:"system"`
	}
	readJSONFile(t, filepath.Join(benchDir, "prompts/p0s.json"), &benchmarkPrompt)
	const schemaLine = "\nAnswer with only a JSON object that matches this JSON Schema:\n"
	system, schemaText, found := strings.Cut(benchmarkPrompt.System, schemaLine)
	if !found {
		t.Fatal("the P0s prompt has no schema line")
	}
	prompt := struct {
		System string
		Schema json.RawMessage
	}{system, json.RawMessage(schemaText)}
	var jobs []benchJob
	readJSONFile(t, filepath.Join(benchDir, "eval-jobs.json"), &jobs)
	getBenchmarkAnswers := func(run string) map[string]json.RawMessage {
		var recorded struct {
			Results []struct {
				ID    string          `json:"id"`
				Facts json.RawMessage `json:"facts"`
			} `json:"results"`
		}
		readJSONFile(t, filepath.Join(benchDir, "runs", run+".json"), &recorded)
		answers := map[string]json.RawMessage{}
		for _, result := range recorded.Results {
			answers[result.ID] = result.Facts
		}
		return answers
	}

	runtime := modelruntime.New(modelruntime.Settings{
		LlamaServer: llamaServer, ModelsDir: modelsDir, Port: getFreePort(t), IdleTimeout: 3 * time.Second,
		LogPath: filepath.Join(t.TempDir(), "llama-server.log"),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runtime.Run(ctx)

	ask := func(model string, job benchJob) (json.RawMessage, time.Duration) {
		t.Helper()
		baseURL, release, err := runtime.Acquire(ctx, model)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		started := time.Now()
		answer, err := chatcompletions.NewClient(baseURL).CompleteJSON(ctx, chatcompletions.JSONRequest{
			Model: model, System: prompt.System, User: formatBenchJob(job), SchemaName: "job_facts", Schema: prompt.Schema, MaxTokens: 2048,
		})
		if err != nil {
			t.Fatalf("%s on %s: %v", model, job.ID, err)
		}
		return answer.Object, time.Since(started)
	}
	isSameAnswer := func(left, right json.RawMessage) bool {
		var leftValue, rightValue any
		return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
	}

	var tested []benchJob
	for _, job := range jobs {
		if job.Split == "test" && len(tested) < 3 {
			tested = append(tested, job)
		}
	}
	for _, run := range []struct{ model, benchmark string }{
		{"Qwen3.8-27B-Q4_K_M.gguf", "qwen38-27b-q4-p0s"},
		{"Qwen3.5-9B-Q4_K_M.gguf", "qwen35-9b-q4-p0s"},
	} {
		recorded := getBenchmarkAnswers(run.benchmark)
		for _, job := range tested {
			answer, took := ask(run.model, job)
			if !isSameAnswer(answer, recorded[job.ID]) {
				t.Errorf("%s on %s differs from the benchmark:\n got  %s\n want %s", run.model, job.ID, answer, recorded[job.ID])
			}
			t.Logf("%s on %s: %s, same as the benchmark", run.model, job.ID[:8], took.Round(100*time.Millisecond))
		}
		if status := runtime.Status(); status.Model != run.model {
			t.Fatalf("after %s, the runtime holds %q", run.model, status.Model)
		}
	}
	if count := countLlamaServers(t); count != 1 {
		t.Fatalf("%d llama-server processes after the swap; want one", count)
	}
	deadline := time.Now().Add(30 * time.Second)
	for runtime.Status().State != modelruntime.StateStopped && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
	}
	if runtime.Status().State != modelruntime.StateStopped || countLlamaServers(t) != 0 {
		t.Fatalf("the idle runtime is still %s", runtime.Status().State)
	}
}

func countLlamaServers(t *testing.T) int {
	t.Helper()
	output, _ := exec.Command("pgrep", "-f", "llama-server -m").Output()
	return len(strings.Fields(string(output)))
}
