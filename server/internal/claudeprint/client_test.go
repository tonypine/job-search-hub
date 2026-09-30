package claudeprint_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/claudeprint"
)

// writeFakeClaude writes a CLI that saves its arguments, one per line, and
// prints answer.
func writeFakeClaude(t *testing.T, answer string) (string, string) {
	t.Helper()
	folder := t.TempDir()
	argumentsFile := filepath.Join(folder, "arguments")
	script := "#!/bin/sh\nfor argument in \"$@\"; do printf '%s\\n' \"$argument\" >> " + argumentsFile + "; done\ncat <<'ANSWER'\n" + answer + "\nANSWER\n"
	binary := filepath.Join(folder, "claude")
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, argumentsFile
}

func TestAnAnswerComesFromTheStructuredOutputAndTheRunIsRecorded(t *testing.T) {
	binary, argumentsFile := writeFakeClaude(t, `{"is_error":false,"result":"","structured_output":{"match":"strong"},
		"usage":{"input_tokens":10,"cache_read_input_tokens":900,"output_tokens":120},"modelUsage":{"claude-sonnet-5-5":{}}}`)
	var records []chatcompletions.RunRecord
	client := &claudeprint.Client{Binary: binary, Directory: t.TempDir(), Model: "sonnet",
		RecordRun: func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }}

	answer, err := client.CompleteJSON(context.Background(), chatcompletions.JSONRequest{
		System: "Brief it.", User: "Title: Engineer", SchemaName: "job_brief", Schema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil || string(answer.Object) != `{"match":"strong"}` || answer.Model != "claude-sonnet-5-5" {
		t.Fatalf("answer = %s %s, %v", answer.Object, answer.Model, err)
	}
	arguments, _ := os.ReadFile(argumentsFile)
	for _, want := range []string{"-p\nTitle: Engineer\n", "--system-prompt\nBrief it.\n", "--json-schema\n{\"type\":\"object\"}\n", "--tools\n\n", "--model\nsonnet\n", "--strict-mcp-config\n"} {
		if !strings.Contains(string(arguments), want) {
			t.Errorf("arguments lack %q:\n%s", want, arguments)
		}
	}
	if len(records) != 1 || records[0].Outcome != chatcompletions.RunSucceeded || records[0].Kind != "job_brief" || records[0].BaseURL != claudeprint.BaseURL ||
		records[0].PromptTokens != 910 || records[0].CompletionTokens != 120 || records[0].Model != "claude-sonnet-5-5" {
		t.Errorf("records = %+v", records)
	}
}

func TestAnAnswerWithoutStructuredOutputIsInvalid(t *testing.T) {
	binary, _ := writeFakeClaude(t, `{"is_error":true,"result":"Credit balance too low"}`)
	var records []chatcompletions.RunRecord
	client := &claudeprint.Client{Binary: binary, Directory: t.TempDir(),
		RecordRun: func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }}

	_, err := client.CompleteJSON(context.Background(), chatcompletions.JSONRequest{SchemaName: "job_brief", Schema: json.RawMessage(`{}`)})
	if !errors.Is(err, chatcompletions.ErrInvalidAnswer) || !strings.Contains(err.Error(), "Credit balance too low") {
		t.Fatalf("error = %v", err)
	}
	if len(records) != 1 || records[0].Outcome != chatcompletions.RunInvalid {
		t.Errorf("records = %+v", records)
	}
}

func TestAMissingBinaryFails(t *testing.T) {
	client := &claudeprint.Client{Binary: filepath.Join(t.TempDir(), "absent"), Directory: t.TempDir()}
	if _, err := client.CompleteJSON(context.Background(), chatcompletions.JSONRequest{}); err == nil || errors.Is(err, chatcompletions.ErrInvalidAnswer) {
		t.Fatalf("error = %v, want a failure", err)
	}
}
