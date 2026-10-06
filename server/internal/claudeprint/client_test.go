package claudeprint_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/claudeprint"
	"github.com/tonypine/job-search-hub/server/internal/drain"
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

func TestNoRunStartsWhileTheHubDrains(t *testing.T) {
	binary, argumentsFile := writeFakeClaude(t, `{"structured_output":{"match":"strong"}}`)
	hub := drain.New(time.Hour)
	var records []chatcompletions.RunRecord
	client := &claudeprint.Client{Binary: binary, Directory: t.TempDir(), Drain: hub,
		RecordRun: func(_ context.Context, record chatcompletions.RunRecord) { records = append(records, record) }}

	hub.Start()
	_, err := client.CompleteJSON(context.Background(), chatcompletions.JSONRequest{SchemaName: "job_brief", Schema: json.RawMessage(`{}`)})
	if !errors.Is(err, drain.ErrDraining) || !errors.Is(err, chatcompletions.ErrUnreachable) {
		t.Fatalf("error = %v, want ErrDraining read as unreachable", err)
	}
	if _, statErr := os.Stat(argumentsFile); !errors.Is(statErr, os.ErrNotExist) || len(records) != 0 {
		t.Fatalf("the CLI ran while draining: %v, records %+v", statErr, records)
	}

	hub.Cancel()
	if _, err := client.CompleteJSON(context.Background(), chatcompletions.JSONRequest{SchemaName: "job_brief", Schema: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("after the drain: %v", err)
	}
}

func TestARunningRunIsListedAndFinishesWhileTheHubDrains(t *testing.T) {
	folder := t.TempDir()
	started, proceed := filepath.Join(folder, "started"), filepath.Join(folder, "proceed")
	script := "#!/bin/sh\ntouch " + started + "\nwhile [ ! -f " + proceed + " ]; do sleep 0.02; done\necho '{\"structured_output\":{\"match\":\"strong\"}}'\n"
	binary := filepath.Join(folder, "claude")
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	hub := drain.New(time.Hour)
	client := &claudeprint.Client{Binary: binary, Directory: t.TempDir(), Drain: hub}
	jobID := uuid.New()

	answered := make(chan error, 1)
	go func() {
		_, err := client.CompleteJSON(context.Background(), chatcompletions.JSONRequest{
			SchemaName: "job_brief", Schema: json.RawMessage(`{}`), Task: chatcompletions.TaskLabel{SubjectID: &jobID},
		})
		answered <- err
	}()
	waitForFile(t, started)
	hub.Start()
	running := hub.Running()
	if len(running) != 1 || running[0].Type != drain.TypeClaudeRun || running[0].Kind != "job_brief" || running[0].Subject != jobID.String() {
		t.Fatalf("running = %+v", running)
	}

	if err := os.WriteFile(proceed, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-answered:
		if err != nil {
			t.Fatalf("the running run failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the running run didn't finish")
	}
	if running := hub.Running(); len(running) != 0 {
		t.Fatalf("still listed: %+v", running)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	for range 500 {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}
