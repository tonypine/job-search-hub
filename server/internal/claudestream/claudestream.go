// Package claudestream reads the stream-json output of `claude -p` and checks
// that an agent session ran isolated: nothing loaded beyond the hub.
package claudestream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

type Plugin struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type MCPServer struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Init is the session's startup report: everything it loaded.
type Init struct {
	SessionID     string      `json:"session_id"`
	Model         string      `json:"model"`
	Plugins       []Plugin    `json:"plugins"`
	Skills        []string    `json:"skills"`
	SlashCommands []string    `json:"slash_commands"`
	MCPServers    []MCPServer `json:"mcp_servers"`
	Tools         []string    `json:"tools"`
}

type ToolUse struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Result is the session's final report. The cost stays a json.Number so the
// decimal Claude reported is never rounded through a float.
type Result struct {
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	SessionID        string          `json:"session_id"`
	TotalCostUSD     json.Number     `json:"total_cost_usd"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	Text             string          `json:"result"`
}

// Message is one decoded line of the stream. Exactly one of Init, Result or
// the assistant content is set, depending on Type; hook events set IsHook.
type Message struct {
	Type     string
	Subtype  string
	Init     *Init
	Result   *Result
	ToolUses []ToolUse
	Texts    []string
	IsHook   bool
}

// Decode reads one line of stream-json output.
func Decode(line []byte) (Message, error) {
	var envelope struct {
		Type    string          `json:"type"`
		Subtype string          `json:"subtype"`
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return Message{}, fmt.Errorf("decode a stream line: %w", err)
	}
	decoded := Message{Type: envelope.Type, Subtype: envelope.Subtype}

	switch {
	case envelope.Type == "system" && envelope.Subtype == "init":
		decoded.Init = &Init{}
		return decoded, json.Unmarshal(line, decoded.Init)
	case envelope.Type == "system" && strings.HasPrefix(envelope.Subtype, "hook_"):
		decoded.IsHook = true
	case envelope.Type == "result":
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		decoded.Result = &Result{}
		return decoded, decoder.Decode(decoded.Result)
	case envelope.Type == "assistant":
		var content struct {
			Content []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		}
		if err := json.Unmarshal(envelope.Message, &content); err != nil {
			return Message{}, fmt.Errorf("decode an assistant message: %w", err)
		}
		for _, block := range content.Content {
			switch block.Type {
			case "tool_use":
				decoded.ToolUses = append(decoded.ToolUses, ToolUse{Name: block.Name, Input: block.Input})
			case "text":
				decoded.Texts = append(decoded.Texts, block.Text)
			}
		}
	}
	return decoded, nil
}

// allowedBuiltInTools are the only built-in tools a triage session may have;
// everything else must come from the hub's own MCP server.
var allowedBuiltInTools = []string{"StructuredOutput", "WebFetch", "WebSearch"}

// CheckIsolation returns an error naming everything the session loaded beyond
// Claude Code's built-in plugins, the allowed tools and the hub's MCP server
// (named mcpServerName), or naming the hub when it is not connected.
func CheckIsolation(init Init, mcpServerName string) error {
	var problems []string
	for _, plugin := range init.Plugins {
		if !strings.HasSuffix(plugin.Source, "@builtin") {
			problems = append(problems, "plugin "+plugin.Source)
		}
	}
	for _, skill := range init.Skills {
		problems = append(problems, "skill "+skill)
	}
	for _, command := range init.SlashCommands {
		problems = append(problems, "slash command "+command)
	}

	hubConnected := false
	for _, server := range init.MCPServers {
		if server.Name != mcpServerName {
			problems = append(problems, "MCP server "+server.Name)
			continue
		}
		hubConnected = server.Status == "connected"
	}
	if !hubConnected {
		problems = append(problems, "the hub's MCP server is not connected")
	}

	hubToolPrefix := "mcp__" + mcpServerName + "__"
	for _, tool := range init.Tools {
		if !slices.Contains(allowedBuiltInTools, tool) && !strings.HasPrefix(tool, hubToolPrefix) {
			problems = append(problems, "tool "+tool)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("the session is not isolated: %s", strings.Join(problems, ", "))
	}
	return nil
}

// FindForbiddenFetch returns the URL of a WebFetch aimed at LinkedIn, which
// agents must never fetch; ok is false for any other tool use.
func FindForbiddenFetch(toolUse ToolUse) (string, bool) {
	if toolUse.Name != "WebFetch" {
		return "", false
	}
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(toolUse.Input, &input); err != nil {
		return "", false
	}
	parsed, err := url.Parse(input.URL)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com") {
		return input.URL, true
	}
	return "", false
}
