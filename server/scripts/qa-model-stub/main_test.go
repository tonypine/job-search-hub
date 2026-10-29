package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/jobbriefs"
	"github.com/tonypine/job-search-hub/server/internal/modelrouter"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

// checkFits fails the test unless answer matches schema.
func checkFits(t *testing.T, name string, schema, answer json.RawMessage) {
	t.Helper()
	var parsed jsonschema.Schema
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("%s: read the schema: %v", name, err)
	}
	resolved, err := parsed.Resolve(nil)
	if err != nil {
		t.Fatalf("%s: resolve the schema: %v", name, err)
	}
	var value map[string]any
	if err := json.Unmarshal(answer, &value); err != nil {
		t.Fatalf("%s: the answer %s isn't an object: %v", name, answer, err)
	}
	if err := resolved.Validate(value); err != nil {
		t.Errorf("%s: the answer %s doesn't fit the schema: %v", name, answer, err)
	}
}

func TestEveryPromptsSchemaGetsAnAnswerThatFitsIt(t *testing.T) {
	paths, err := filepath.Glob("../../internal/testdatabase/agentprompts/*.schema.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no schemas: %v", err)
	}
	for _, path := range paths {
		schema, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		answer, err := buildAnswer(schema)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		checkFits(t, filepath.Base(path), schema, answer)
	}
}

func TestAnAnswerKeepsToItsSchemasLimitsAndReferences(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"additionalProperties": false,
		"required": ["points", "none", "code", "count", "ratio", "kind", "later", "when", "flag", "tags"],
		"$defs": {"point": {"type": "object", "required": ["text"], "properties": {"text": {"type": "string", "minLength": 30}}}},
		"properties": {
			"points": {"type": "array", "minItems": 2, "items": {"$ref": "#/$defs/point"}},
			"none": {"type": "array", "maxItems": 0, "items": {"type": "string"}},
			"code": {"type": "string", "maxLength": 2},
			"count": {"type": "integer", "minimum": 3},
			"ratio": {"type": "number", "exclusiveMinimum": 0.5},
			"kind": {"enum": ["first", "second"]},
			"later": {"anyOf": [{"type": "null"}, {"type": "string"}]},
			"when": {"type": ["null", "string"], "format": "date-time"},
			"flag": {"const": true},
			"tags": {"type": "array", "items": {"type": "string"}}
		}
	}`)
	answer, err := buildAnswer(schema)
	if err != nil {
		t.Fatal(err)
	}
	checkFits(t, "limits", schema, answer)
	var got struct {
		Points []struct{ Text string } `json:"points"`
		Kind   string                  `json:"kind"`
		Later  *string                 `json:"later"`
		Tags   []string                `json:"tags"`
	}
	if err := json.Unmarshal(answer, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Points) != 2 || len(got.Points[0].Text) != 30 || got.Kind != "first" || got.Later == nil || len(got.Tags) != 1 || got.Tags[0] != standInText {
		t.Errorf("answer = %s", answer)
	}
}

func TestARequestWithNoSchemaGetsAnEmptyObject(t *testing.T) {
	answer, err := buildAnswer(nil)
	if err != nil || string(answer) != `{}` {
		t.Errorf("buildAnswer(nil) = %s, %v", answer, err)
	}
}

func TestTheChatCompletionsClientGetsItsAnswerAfterTheDelay(t *testing.T) {
	const delay = 200 * time.Millisecond
	server := httptest.NewServer(newHandler(delay))
	defer server.Close()
	schema, err := os.ReadFile("../../internal/testdatabase/agentprompts/job_brief.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	client := chatcompletions.NewClient(server.URL + "/v1")
	var outcome string
	client.RecordRun = func(_ context.Context, record chatcompletions.RunRecord) { outcome = record.Outcome }
	started := time.Now()
	answer, err := client.CompleteJSON(context.Background(), chatcompletions.JSONRequest{
		Model: Model, System: "Brief it.", User: "A job.", SchemaName: "job_brief", Schema: schema, MaxTokens: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < delay {
		t.Errorf("answered after %s, before the %s delay", elapsed, delay)
	}
	if outcome != chatcompletions.RunSucceeded || answer.Model != Model {
		t.Errorf("outcome %q, model %q", outcome, answer.Model)
	}
	checkFits(t, "job_brief", schema, answer.Object)
}

// A provider that doesn't enforce schemas is sent the schema in the system
// prompt only, and the client checks the answer against it.
func TestAnAnswerFitsTheSchemaInTheSystemPromptWhenTheProviderDoesntEnforceIt(t *testing.T) {
	server := httptest.NewServer(newHandler(0))
	defer server.Close()
	schema, err := os.ReadFile("../../internal/testdatabase/agentprompts/job_facts.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, err := chatcompletions.NewClient(server.URL+"/v1").CompleteJSON(context.Background(), chatcompletions.JSONRequest{
		Model: Model, System: "Read the facts.", User: "A job.", SchemaName: "job_facts", Schema: schema, SchemaNotEnforced: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var facts struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(answer.Object, &facts); err != nil || facts.Summary != standInText {
		t.Errorf("answer = %s, %v", answer.Object, err)
	}
}

func TestARequestItsCallerLeftEndsWithoutWaitingOutTheDelay(t *testing.T) {
	server := httptest.NewServer(newHandler(time.Minute))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := chatcompletions.NewClient(server.URL+"/v1").CompleteJSON(ctx, chatcompletions.JSONRequest{
		Model: Model, SchemaName: "job_brief", Schema: json.RawMessage(`{"type":"object"}`),
	})
	if !errors.Is(err, chatcompletions.ErrUnreachable) {
		t.Errorf("err = %v, want the request to fail as the caller left", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("took %s", elapsed)
	}
}

type noRates struct{}

func (noRates) GetRates(context.Context, string) (map[string]float64, error) { return nil, nil }

// A full brief routed to the stub, as dev-server-seed routes it, is written
// through the router and recorded as a run that succeeded on the stub.
func TestAFullBriefRoutedToTheStubLandsThroughTheRouter(t *testing.T) {
	ctx := context.Background()
	hub := store.New(testdatabase.New(t))
	owner := store.Actor{Kind: store.ActorOwner}
	server := httptest.NewServer(newHandler(50 * time.Millisecond))
	defer server.Close()
	provider, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "QA stand-in model", BaseURL: server.URL + "/v1", EnforcesSchema: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTaskRoute(ctx, owner, store.RunKindJobBriefFull, store.TaskRouteInput{ProviderID: provider.ID, Model: Model}); err != nil {
		t.Fatal(err)
	}
	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Senior Product Engineer", URL: "https://acme.example/jobs/1", Description: "Build."})
	if err != nil {
		t.Fatal(err)
	}

	router := modelrouter.New(hub)
	router.RecordRun = func(ctx context.Context, record chatcompletions.RunRecord) {
		if _, err := hub.RecordTaskRun(ctx, store.NewTaskRun{
			ID: record.ID, Kind: record.Kind, SubjectID: record.Task.SubjectID, BaseURL: record.BaseURL, Provider: record.Provider, Model: record.Model,
			PromptID: record.Task.PromptID, PromptVersion: record.Task.PromptVersion, InputHash: record.InputHash, Output: record.Output,
			StartedAt: record.StartedAt, Duration: record.Duration, Outcome: record.Outcome, Error: record.Error, StartedByOwner: record.StartedByOwner,
		}); err != nil {
			t.Errorf("record the run: %v", err)
		}
	}
	writer := jobbriefs.NewWriter(hub, router, noRates{})
	writer.FullClient = router
	if err := writer.WriteFullBrief(ctx, job.ID); err != nil {
		t.Fatal(err)
	}

	page, err := hub.ListRuns(ctx, store.RunFilter{Kind: store.RunKindJobBriefFull, SubjectID: &job.ID, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Runs) != 1 {
		t.Fatalf("runs = %+v, want one", page.Runs)
	}
	run := page.Runs[0]
	if run.Outcome != chatcompletions.RunSucceeded || run.Model != Model || run.Provider != "QA stand-in model" || !run.StartedByOwner {
		t.Errorf("run = %+v", run)
	}
	brief, err := hub.GetJobBrief(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if brief == nil || brief.Tier != store.JobBriefTierFull || brief.Model != Model || brief.Reason != standInText {
		t.Errorf("brief = %+v", brief)
	}
}
