package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// replyTimeout bounds one draft; it needs a single turn and no tools.
const replyTimeout = 3 * time.Minute

// draftRecruiterReply drafts a message back to the recruiter who started a
// conversation, from the prompt the hub fills in, and prints it. The draft
// runs in a `claude -p` session with no tools and no MCP servers: it only
// writes text.
func draftRecruiterReply(ctx context.Context, config cliConfig, conversationID, model string, out io.Writer) error {
	var rendered struct {
		Prompt string `json:"prompt"`
	}
	if err := getJSON(ctx, config, "/v1/recruiters/"+url.PathEscape(conversationID)+"/reply-prompt", &rendered); err != nil {
		return err
	}
	workingDirectory, err := agentWorkingDirectory()
	if err != nil {
		return err
	}
	claudeBinary := os.Getenv("HUB_CLAUDE_BIN")
	if claudeBinary == "" {
		claudeBinary = "claude"
	}
	sessionCtx, cancel := context.WithTimeout(ctx, replyTimeout)
	defer cancel()
	command := exec.CommandContext(sessionCtx, claudeBinary, buildDraftArguments(rendered.Prompt, model)...)
	command.Dir = workingDirectory
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("claude exited: %v: %s", err, lastLines(strings.TrimSpace(stderr.String()+"\n"+string(output)), 5))
	}
	var result struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("read claude's answer: %w", err)
	}
	if result.IsError || strings.TrimSpace(result.Result) == "" {
		return errors.New("claude wrote no draft: " + result.Result)
	}
	_, err = fmt.Fprintln(out, strings.TrimSpace(result.Result))
	return err
}

// buildDraftArguments runs a one-turn session that can only write: no tools,
// no MCP servers, no user settings or hooks.
func buildDraftArguments(prompt, model string) []string {
	arguments := []string{
		"-p", prompt,
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
