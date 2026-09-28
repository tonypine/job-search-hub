package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/claudestream"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// hubMCPServerName is the name the hub's MCP server gets inside an agent
// session, so its tools are called mcp__hub__<tool>.
const hubMCPServerName = "hub"

const maximumStreamLineBytes = 16 << 20

type triageOptions struct {
	model  string
	effort string
}

// sessionOutcome is how an agent session ended, in the shape the hub's finish
// route records.
type sessionOutcome struct {
	Status          string          `json:"status"`
	ClaudeSessionID string          `json:"claude_session_id,omitempty"`
	CostUSDEstimate json.Number     `json:"cost_usd_estimate,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           string          `json:"error,omitempty"`
}

// buildClaudeArguments isolates the session: no user settings, plugins,
// skills or other MCP servers, and no built-in tools but the web ones. The
// hub's tools are its only way to write.
func buildClaudeArguments(prompt, resultSchema, mcpConfigPath string, options triageOptions) []string {
	arguments := []string{
		"-p", prompt,
		"--output-format", "stream-json", "--verbose",
		"--json-schema", resultSchema,
		"--mcp-config", mcpConfigPath, "--strict-mcp-config",
		"--setting-sources", "project",
		"--disable-slash-commands",
		"--tools", "WebSearch,WebFetch",
		"--allowedTools", "WebSearch,WebFetch,mcp__" + hubMCPServerName + "__*",
		"--disallowedTools", "WebFetch(domain:linkedin.com)",
		"--permission-mode", "dontAsk",
		"--permission-prompts", "none",
	}
	if options.model != "" {
		arguments = append(arguments, "--model", options.model)
	}
	if options.effort != "" {
		arguments = append(arguments, "--effort", options.effort)
	}
	return arguments
}

// writeMCPConfig writes the session's MCP config, which carries the run's
// token, to a file only this user can read. The caller removes it.
func writeMCPConfig(hubURL, runToken string) (string, error) {
	config := map[string]any{"mcpServers": map[string]any{
		hubMCPServerName: map[string]any{
			"type":    "http",
			"url":     strings.TrimSuffix(hubURL, "/") + "/mcp",
			"headers": map[string]string{"Authorization": "Bearer " + runToken},
		},
	}}
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "hub-mcp-*.json")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(encoded); err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

// agentWorkingDirectory is one fixed, empty directory for every session, so
// Claude Code does not open a new project folder of its own per run.
func agentWorkingDirectory() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(cache, "job-search-hub", "agent-workdir")
	return directory, os.MkdirAll(directory, 0o700)
}

// runTriageSession runs the agent in an isolated `claude -p` session, prints
// its progress, and reports how it ended. A session that loaded anything
// beyond the hub is stopped as soon as its startup report arrives.
func runTriageSession(ctx context.Context, hubURL string, started startedRun, options triageOptions, out io.Writer) sessionOutcome {
	failed := func(reason string) sessionOutcome { return sessionOutcome{Status: store.AgentRunFailed, Error: reason} }

	mcpConfigPath, err := writeMCPConfig(hubURL, started.Token)
	if err != nil {
		return failed("write the MCP config: " + err.Error())
	}
	defer os.Remove(mcpConfigPath)
	workingDirectory, err := agentWorkingDirectory()
	if err != nil {
		return failed("prepare the working directory: " + err.Error())
	}

	sessionCtx, stopSession := context.WithCancel(ctx)
	defer stopSession()
	claudeBinary := os.Getenv("HUB_CLAUDE_BIN")
	if claudeBinary == "" {
		claudeBinary = "claude"
	}
	command := exec.CommandContext(sessionCtx, claudeBinary,
		buildClaudeArguments(started.Prompt, string(started.ResultSchema), mcpConfigPath, options)...)
	command.Dir = workingDirectory
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return failed(err.Error())
	}
	if err := command.Start(); err != nil {
		return failed("start claude: " + err.Error())
	}

	var outcome *sessionOutcome
	var warnings []string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maximumStreamLineBytes)
	for scanner.Scan() {
		message, err := claudestream.Decode(scanner.Bytes())
		if err != nil {
			continue
		}
		switch {
		case message.IsHook:
			isolationFailure := failed("the session is not isolated: a hook ran (" + message.Subtype + ")")
			outcome = &isolationFailure
			stopSession()
		case message.Init != nil:
			if err := claudestream.CheckIsolation(*message.Init, hubMCPServerName); err != nil {
				isolationFailure := failed(err.Error())
				outcome = &isolationFailure
				stopSession()
			}
		case message.Result != nil && outcome == nil:
			finished := sessionOutcome{
				Status:          store.AgentRunSucceeded,
				ClaudeSessionID: message.Result.SessionID,
				CostUSDEstimate: message.Result.TotalCostUSD,
				Result:          message.Result.StructuredOutput,
			}
			if message.Result.IsError || len(message.Result.StructuredOutput) == 0 {
				finished.Status = store.AgentRunFailed
				finished.Error = "the session ended without a result: " + message.Result.Subtype + " " + message.Result.Text
			}
			outcome = &finished
		}
		for _, toolUse := range message.ToolUses {
			if forbiddenURL, found := claudestream.FindForbiddenFetch(toolUse); found {
				warnings = append(warnings, "tried to fetch "+forbiddenURL+" (blocked)")
			}
			printToolUse(out, toolUse)
		}
	}
	waitErr := command.Wait()

	if outcome == nil {
		reason := "the session ended without a result"
		if ctx.Err() != nil {
			reason = "the session was stopped: " + ctx.Err().Error()
		} else if waitErr != nil {
			reason = fmt.Sprintf("claude exited: %v: %s", waitErr, lastLines(stderr.String(), 5))
		}
		noResult := failed(reason)
		outcome = &noResult
	}
	if len(warnings) > 0 {
		outcome.Error = strings.TrimSpace(outcome.Error + "; " + strings.Join(warnings, "; "))
		fmt.Fprintln(out, "Warning: "+strings.Join(warnings, "; "))
	}
	return *outcome
}

func printToolUse(out io.Writer, toolUse claudestream.ToolUse) {
	name := strings.TrimPrefix(toolUse.Name, "mcp__"+hubMCPServerName+"__")
	input := string(toolUse.Input)
	if len(input) > 100 {
		input = input[:100] + "…"
	}
	fmt.Fprintf(out, "  · %s %s\n", name, input)
}

func lastLines(text string, count int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, " | ")
}
