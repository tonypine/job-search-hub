package chatcompletions_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	if err != nil || string(answer) != `{"stack":"Go"}` {
		t.Fatalf("answer = %s, %v", answer, err)
	}
	format := (*received)["response_format"].(map[string]any)
	schema := format["json_schema"].(map[string]any)
	if (*received)["model"] != "qwen/qwen3.5-9b" || format["type"] != "json_schema" || schema["name"] != "job_facts" || schema["strict"] != true || schema["schema"] == nil {
		t.Fatalf("request = %+v", *received)
	}
}

func TestAnEmptyContentFallsBackToReasoningContent(t *testing.T) {
	client, _ := answerWith(t, http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"content":"","reasoning_content":"{\"stack\":\"Go\"}"}}]}`)

	if answer, err := client.CompleteJSON(context.Background(), request); err != nil || string(answer) != `{"stack":"Go"}` {
		t.Fatalf("answer = %s, %v", answer, err)
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
		if _, err := client.CompleteJSON(context.Background(), request); err == nil || !strings.Contains(err.Error(), answer.want) {
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
