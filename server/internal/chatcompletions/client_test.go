package chatcompletions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
)

var request = chatcompletions.JSONRequest{
	Model: "qwen/qwen3.5-9b", System: "Record the facts.", User: "Title: Engineer", SchemaName: "job_facts",
	Schema: json.RawMessage(`{"type":"object","properties":{"stack":{"type":"string"}}}`), MaxTokens: 100,
}

func answerWith(t *testing.T, status int, body string) (*chatcompletions.Client, *map[string]any) {
	t.Helper()
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		payload, _ := io.ReadAll(r.Body)
		json.Unmarshal(payload, &received)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return chatcompletions.NewClient(server.URL + "/v1/"), &received
}

func TestTheAnswerComesFromContentAndTheRequestCarriesTheSchema(t *testing.T) {
	client, received := answerWith(t, http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"stack\":\"Go\"}"}}]}`)

	answer, err := client.CompleteJSON(context.Background(), request)
	if err != nil || string(answer.Object) != `{"stack":"Go"}` || answer.Model != "qwen/qwen3.5-9b" {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	format := (*received)["response_format"].(map[string]any)
	schema := format["json_schema"].(map[string]any)
	if (*received)["model"] != "qwen/qwen3.5-9b" || format["type"] != "json_schema" || schema["name"] != "job_facts" || schema["strict"] != true || schema["schema"] == nil {
		t.Fatalf("request = %+v", *received)
	}
	system := (*received)["messages"].([]any)[0].(map[string]any)["content"]
	if system != "Record the facts.\n\nAnswer with only a JSON object that matches this JSON Schema:\n"+string(request.Schema) {
		t.Fatalf("system message = %q", system)
	}
}

func TestAnEmptyContentFallsBackToReasoningContent(t *testing.T) {
	client, _ := answerWith(t, http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"","reasoning_content":"{\"stack\":\"Go\"}"}}]}`)

	if answer, err := client.CompleteJSON(context.Background(), request); err != nil || string(answer.Object) != `{"stack":"Go"}` {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
}

func TestFailedAnswersAreErrors(t *testing.T) {
	for name, answer := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"server error":   {http.StatusBadRequest, `{"error":"'type' must be a string"}`, "'type' must be a string"},
		"not an object":  {http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"[1]"}}]}`, "not a JSON object"},
		"cut off":        {http.StatusOK, `{"choices":[{"finish_reason":"length","message":{"content":"{\"stack\":"}}]}`, "cut off"},
		"no choices":     {http.StatusOK, `{"choices":[]}`, "no choices"},
		"not a response": {http.StatusOK, `<html>`, "read the completion"},
	} {
		client, _ := answerWith(t, answer.status, answer.body)
		if _, err := client.CompleteJSON(context.Background(), request); err == nil || !strings.Contains(err.Error(), answer.want) || errors.Is(err, chatcompletions.ErrInvalidAnswer) != (name == "not an object" || name == "cut off") {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, answer.want)
		}
	}
}

func TestAServerThatIsNotRunningIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	if _, err := chatcompletions.NewClient(server.URL).CompleteJSON(context.Background(), request); !errors.Is(err, chatcompletions.ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
}

func TestEveryRequestIsRecordedWithItsOutcome(t *testing.T) {
	var records []chatcompletions.RunRecord
	record := func(_ context.Context, run chatcompletions.RunRecord) { records = append(records, run) }

	succeeding, _ := answerWith(t, http.StatusOK,
		`{"choices":[{"finish_reason":"stop","message":{"content":"{\"stack\":\"Go\"}"}}],"usage":{"prompt_tokens":120,"completion_tokens":8}}`)
	succeeding.RecordRun = record
	labeled := request
	labeled.Task.PromptVersion = 3
	if _, err := succeeding.CompleteJSON(context.Background(), labeled); err != nil {
		t.Fatal(err)
	}
	invalid, _ := answerWith(t, http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"not json"}}]}`)
	invalid.RecordRun = record
	invalid.CompleteJSON(context.Background(), request)
	unreachable := chatcompletions.NewClient("http://127.0.0.1:1/v1")
	unreachable.RecordRun = record
	unreachable.CompleteJSON(context.Background(), request)

	if len(records) != 3 {
		t.Fatalf("records = %d, want 3", len(records))
	}
	first := records[0]
	if first.Outcome != chatcompletions.RunSucceeded || first.Kind != "job_facts" || first.Model != "qwen/qwen3.5-9b" || first.PromptTokens != 120 ||
		first.CompletionTokens != 8 || string(first.Output) != `{"stack":"Go"}` || first.Task.PromptVersion != 3 || len(first.InputHash) != 64 {
		t.Errorf("success = %+v", first)
	}
	if records[1].Outcome != chatcompletions.RunInvalid || records[1].Output != nil || records[1].Error == "" {
		t.Errorf("invalid = %+v", records[1])
	}
	if records[2].Outcome != chatcompletions.RunFailed || !strings.Contains(records[2].Error, "unreachable") {
		t.Errorf("unreachable = %+v", records[2])
	}
	if records[0].InputHash != records[1].InputHash {
		t.Error("the same input hashed differently")
	}
}

func TestARunIsRecordedUnderItsTasksKindAndProviderAndWhoAskedForIt(t *testing.T) {
	var records []chatcompletions.RunRecord
	client, _ := answerWith(t, http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}]}`)
	client.Provider = "LM Studio"
	client.RecordRun = func(_ context.Context, run chatcompletions.RunRecord) { records = append(records, run) }
	full := request
	full.Task.Kind = "job_brief_full"
	if _, err := client.CompleteJSON(chatcompletions.WithStartedByOwner(context.Background()), full); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CompleteJSON(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Kind != "job_brief_full" || !records[0].StartedByOwner || records[0].Provider != "LM Studio" ||
		records[1].Kind != request.SchemaName || records[1].StartedByOwner {
		t.Errorf("records = %+v", records)
	}
}

func TestAKeyIsSentAsABearerToken(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"stack\":\"Go\"}"}}]}`))
	}))
	t.Cleanup(server.Close)
	client := chatcompletions.NewClient(server.URL)
	client.APIKey = "sk-test"

	if _, err := client.CompleteJSON(context.Background(), request); err != nil || authorization != "Bearer sk-test" {
		t.Fatalf("authorization = %q, %v", authorization, err)
	}
}

func TestAServerThatCannotEnforceTheSchemaHasItsAnswerValidated(t *testing.T) {
	unenforced := request
	unenforced.SchemaNotEnforced = true
	unenforced.Schema = json.RawMessage(`{"type":"object","required":["stack"],"properties":{"stack":{"type":"string"}}}`)

	client, received := answerWith(t, http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"`+"```json\\n"+`{\"stack\":\"Go\"}`+"\\n```"+`"}}]}`)
	answer, err := client.CompleteJSON(context.Background(), unenforced)
	if err != nil || string(answer.Object) != `{"stack":"Go"}` {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if format := (*received)["response_format"].(map[string]any); format["type"] != "json_object" {
		t.Fatalf("response_format = %v", format)
	}

	mismatched, _ := answerWith(t, http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"stack\":3}"}}]}`)
	if _, err := mismatched.CompleteJSON(context.Background(), unenforced); !errors.Is(err, chatcompletions.ErrInvalidAnswer) {
		t.Fatalf("a mismatched answer gave %v", err)
	}
}

func TestWorkedExamplesAreSentAsEarlierTurns(t *testing.T) {
	client, received := answerWith(t, http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"stack\":\"Go\"}"}}]}`)
	withExamples := request
	for index := range 4 {
		withExamples.Examples = append(withExamples.Examples, chatcompletions.Example{Input: fmt.Sprintf("Title: Example %d", index), Answer: json.RawMessage(`{"stack":"Rust"}`)})
	}

	if _, err := client.CompleteJSON(context.Background(), withExamples); err != nil {
		t.Fatal(err)
	}
	messages := (*received)["messages"].([]any)
	if len(messages) != 10 {
		t.Fatalf("%d messages, want the system prompt, 4 example pairs and the input", len(messages))
	}
	roles := ""
	for _, message := range messages {
		roles += message.(map[string]any)["role"].(string)[:1]
	}
	second := messages[2].(map[string]any)
	last := messages[9].(map[string]any)
	if roles != "suauauauau" || second["content"] != `{"stack":"Rust"}` || last["content"] != request.User {
		t.Fatalf("roles = %s, second = %v, last = %v", roles, second, last)
	}
}

func TestTheSchemaReachesTheModelInTheOrderItWasWritten(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}]}`))
	}))
	t.Cleanup(server.Close)
	ordered := request
	ordered.Schema = json.RawMessage(`{"type":"object","properties":{"reason":{"type":"string"},"posting_says":{"type":"string"},"answer":{"type":"string"}}}`)

	if _, err := chatcompletions.NewClient(server.URL+"/v1/").CompleteJSON(context.Background(), ordered); err != nil {
		t.Fatal(err)
	}
	format := body[strings.Index(body, `"response_format"`):]
	if !(strings.Index(format, `"reason"`) < strings.Index(format, `"posting_says"`) && strings.Index(format, `"posting_says"`) < strings.Index(format, `"answer"`)) {
		t.Errorf("response_format = %s, want reason, posting_says, answer as written", format)
	}
}

func TestHeadersACostAndTheServersOwnErrorsAreForAServerThatAsksForThem(t *testing.T) {
	var headers http.Header
	var received map[string]any
	status, body := http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"stack\":\"Go\"}"}}],
		"usage":{"prompt_tokens":40,"completion_tokens":8,"cost":0.00012}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers, received = r.Header.Clone(), nil
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer server.Close()
	var records []chatcompletions.RunRecord
	client := chatcompletions.NewClient(server.URL)
	client.Headers = map[string]string{"X-Title": "Job Search Hub"}
	client.AskForCost = true
	client.ReadError = func(status int, payload []byte) error {
		return fmt.Errorf("%w: read %d", chatcompletions.ErrUnreachable, status)
	}
	client.RecordRun = func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }

	if _, err := client.CompleteJSON(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if headers.Get("X-Title") != "Job Search Hub" || fmt.Sprint(received["usage"]) != "map[include:true]" {
		t.Fatalf("headers = %v, body = %v", headers, received)
	}
	if len(records) != 1 || records[0].CostUSD != "0.00012" || records[0].PromptTokens != 40 {
		t.Fatalf("records = %+v", records)
	}

	status, body = http.StatusServiceUnavailable, `{"error":{"code":503,"message":"down"}}`
	if _, err := client.CompleteJSON(context.Background(), request); !errors.Is(err, chatcompletions.ErrUnreachable) || !strings.Contains(err.Error(), "read 503") {
		t.Fatalf("a 503: %v", err)
	}
	status, body = http.StatusOK, `{"error":{"code":502,"message":"the model's provider failed"}}`
	if _, err := client.CompleteJSON(context.Background(), request); !strings.Contains(fmt.Sprint(err), "read 502") {
		t.Fatalf("an error in a 200: %v", err)
	}

	// A server that isn't asked for its cost isn't sent the field.
	plain := chatcompletions.NewClient(server.URL)
	status, body = http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}]}`
	if _, err := plain.CompleteJSON(context.Background(), request); err != nil || received["usage"] != nil {
		t.Fatalf("a plain request: %v, %v", err, received)
	}
}

func TestAPresetRunIDGoesToTheFirstRecordOfItsKindOnly(t *testing.T) {
	id := uuid.New()
	ctx := chatcompletions.WithRunID(context.Background(), "job_brief_full", id)
	if taken := chatcompletions.TakeRunID(ctx, "job_brief"); taken != nil {
		t.Fatalf("another kind took the id: %v", taken)
	}
	if taken := chatcompletions.TakeRunID(ctx, "job_brief_full"); taken == nil || *taken != id {
		t.Fatalf("the first record of the kind got %v, want %s", taken, id)
	}
	if taken := chatcompletions.TakeRunID(ctx, "job_brief_full"); taken != nil {
		t.Fatalf("a second record got the id too: %v", taken)
	}
	if taken := chatcompletions.TakeRunID(context.Background(), "job_brief_full"); taken != nil {
		t.Fatalf("a context with no id gave %v", taken)
	}
}
