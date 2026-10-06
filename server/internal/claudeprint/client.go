// Package claudeprint asks Claude for a JSON answer through the Claude Code
// CLI, `claude -p`, on the owner's own plan: one turn, the answer's schema
// enforced, and no tools, MCP servers, settings or hooks.
package claudeprint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/drain"
)

// BaseURL is what the run record names as the server of a Claude CLI run.
const BaseURL = "claude-cli"

// Client runs the CLI at Binary in Directory, an empty folder of its own so
// no project's instructions reach the model, with Model ("sonnet", or a full
// model id). RecordRun, when set, is told about every run. Drain, when set,
// lists the runs and refuses new ones while the hub drains.
type Client struct {
	Binary    string
	Directory string
	Model     string
	RecordRun func(context.Context, chatcompletions.RunRecord)
	Drain     *drain.Drain
}

// cliAnswer is the part of `claude -p --output-format json` the hub reads.
type cliAnswer struct {
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	Usage            struct {
		InputTokens              int `json:"input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		OutputTokens             int `json:"output_tokens"`
	} `json:"usage"`
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
}

// CompleteJSON asks Claude for the answer request.Schema describes, with
// request.System as its system prompt. Worked examples are left out: Claude
// follows the schema without them. While the hub drains, no run starts: the
// error is drain.ErrDraining, and reads as unreachable, so a pass stops
// rather than counting a failure.
func (client *Client) CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	subject := ""
	if request.Task.SubjectID != nil {
		subject = request.Task.SubjectID.String()
	}
	end, err := client.Drain.Begin(drain.TypeClaudeRun, request.SchemaName, subject)
	if err != nil {
		return chatcompletions.Answer{}, fmt.Errorf("%w: %w", chatcompletions.ErrUnreachable, err)
	}
	defer end()
	record := chatcompletions.RunRecord{
		Kind: request.SchemaName, Task: request.Task, BaseURL: BaseURL, Model: client.Model,
		InputHash: chatcompletions.HashInput(request), StartedAt: time.Now(),
	}
	answer, err := client.run(ctx, request, &record)
	record.Duration = time.Since(record.StartedAt)
	switch {
	case err == nil:
		record.Outcome, record.Output = chatcompletions.RunSucceeded, answer.Object
	case errors.Is(err, chatcompletions.ErrInvalidAnswer):
		record.Outcome, record.Error = chatcompletions.RunInvalid, err.Error()
	default:
		record.Outcome, record.Error = chatcompletions.RunFailed, err.Error()
	}
	if client.RecordRun != nil {
		client.RecordRun(context.WithoutCancel(ctx), record)
	}
	return answer, err
}

func (client *Client) run(ctx context.Context, request chatcompletions.JSONRequest, record *chatcompletions.RunRecord) (chatcompletions.Answer, error) {
	command := exec.CommandContext(ctx, client.Binary, buildArguments(request, client.Model)...)
	command.Dir = client.Directory
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return chatcompletions.Answer{}, fmt.Errorf("claude exited: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	var answer cliAnswer
	if err := json.Unmarshal(output, &answer); err != nil {
		return chatcompletions.Answer{}, fmt.Errorf("%w: claude's output isn't JSON", chatcompletions.ErrInvalidAnswer)
	}
	record.PromptTokens = answer.Usage.InputTokens + answer.Usage.CacheReadInputTokens + answer.Usage.CacheCreationInputTokens
	record.CompletionTokens = answer.Usage.OutputTokens
	for model := range answer.ModelUsage {
		record.Model = model
	}
	if answer.IsError || len(answer.StructuredOutput) == 0 || string(answer.StructuredOutput) == "null" {
		return chatcompletions.Answer{}, fmt.Errorf("%w: claude gave no structured answer: %s", chatcompletions.ErrInvalidAnswer, answer.Result)
	}
	return chatcompletions.Answer{Object: answer.StructuredOutput, Model: record.Model}, nil
}

// buildArguments is a one-turn session that can only answer: every tool,
// MCP server, user setting and slash command is off.
func buildArguments(request chatcompletions.JSONRequest, model string) []string {
	arguments := []string{
		"-p", request.User,
		"--system-prompt", request.System,
		"--json-schema", string(request.Schema),
		"--output-format", "json",
		"--strict-mcp-config",
		"--setting-sources", "project",
		"--disable-slash-commands",
		"--tools", "",
		"--permission-mode", "dontAsk",
	}
	if model != "" {
		arguments = append(arguments, "--model", model)
	}
	return arguments
}
