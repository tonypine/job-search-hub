package claudestream_test

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/claudestream"
)

// readFixture decodes the recorded stream of a real, isolated `claude -p` run
// that called the hub's find_companies and fetched example.com.
func readFixture(t *testing.T) []claudestream.Message {
	t.Helper()
	file, err := os.Open("testdata/triage-stream.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var messages []claudestream.Message
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		message, err := claudestream.Decode(scanner.Bytes())
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		messages = append(messages, message)
	}
	return messages
}

func findInit(t *testing.T, messages []claudestream.Message) claudestream.Init {
	t.Helper()
	for _, message := range messages {
		if message.Init != nil {
			return *message.Init
		}
	}
	t.Fatal("the fixture has no init")
	return claudestream.Init{}
}

func TestTheRecordedStreamDecodes(t *testing.T) {
	messages := readFixture(t)

	var toolNames []string
	var result *claudestream.Result
	for _, message := range messages {
		for _, toolUse := range message.ToolUses {
			toolNames = append(toolNames, toolUse.Name)
		}
		if message.Result != nil {
			result = message.Result
		}
	}
	if strings.Join(toolNames, ",") != "mcp__hub__find_companies,WebFetch,StructuredOutput" {
		t.Fatalf("tool uses = %v", toolNames)
	}
	if result == nil || result.IsError || result.SessionID == "" || result.TotalCostUSD.String() != "0.08809900000000001" {
		t.Fatalf("result = %+v", result)
	}
	var output struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &output); err != nil || output.Answer != "Example Domain" {
		t.Fatalf("structured output = %s", result.StructuredOutput)
	}
}

func TestTheRecordedSessionIsIsolated(t *testing.T) {
	if err := claudestream.CheckIsolation(findInit(t, readFixture(t)), "hub"); err != nil {
		t.Fatal(err)
	}
}

func TestAnythingLoadedBeyondTheHubFailsIsolation(t *testing.T) {
	clean := findInit(t, readFixture(t))
	for name, loaded := range map[string]struct {
		change func(*claudestream.Init)
		named  string
	}{
		"user plugin": {func(init *claudestream.Init) {
			init.Plugins = append(init.Plugins, claudestream.Plugin{Name: "superpowers", Source: "superpowers@claude-plugins-official"})
		}, "superpowers@claude-plugins-official"},
		"skill": {func(init *claudestream.Init) { init.Skills = []string{"brainstorm"} }, "skill brainstorm"},
		"extra MCP": {func(init *claudestream.Init) {
			init.MCPServers = append(init.MCPServers, claudestream.MCPServer{Name: "zendesk", Status: "connected"})
		}, "MCP server zendesk"},
		"built-in tool": {func(init *claudestream.Init) { init.Tools = append(init.Tools, "Bash") }, "tool Bash"},
		"hub not ready": {func(init *claudestream.Init) {
			init.MCPServers = []claudestream.MCPServer{{Name: "hub", Status: "failed"}}
		}, "not connected"},
		"slash commands": {func(init *claudestream.Init) { init.SlashCommands = []string{"groom"} }, "slash command groom"},
	} {
		init := clean
		init.Plugins = append([]claudestream.Plugin(nil), clean.Plugins...)
		init.MCPServers = append([]claudestream.MCPServer(nil), clean.MCPServers...)
		init.Tools = append([]string(nil), clean.Tools...)
		loaded.change(&init)

		err := claudestream.CheckIsolation(init, "hub")
		if err == nil || !strings.Contains(err.Error(), loaded.named) {
			t.Errorf("%s: err = %v, want it to name %q", name, err, loaded.named)
		}
	}
}

func TestLinkedInFetchesAreFlagged(t *testing.T) {
	for rawURL, forbidden := range map[string]bool{
		"https://www.linkedin.com/in/someone": true,
		"https://linkedin.com/company/acme":   true,
		"https://example.com/linkedin.com":    false,
		"https://acme.com/team":               false,
	} {
		input, _ := json.Marshal(map[string]string{"url": rawURL, "prompt": "who works here"})
		_, flagged := claudestream.FindForbiddenFetch(claudestream.ToolUse{Name: "WebFetch", Input: input})
		if flagged != forbidden {
			t.Errorf("%s: flagged = %v, want %v", rawURL, flagged, forbidden)
		}
	}
	if _, flagged := claudestream.FindForbiddenFetch(claudestream.ToolUse{Name: "WebSearch", Input: json.RawMessage(`{"query":"linkedin.com acme"}`)}); flagged {
		t.Error("a web search mentioning LinkedIn was flagged")
	}
}

func TestHookEventsAreRecognized(t *testing.T) {
	message, err := claudestream.Decode([]byte(`{"type":"system","subtype":"hook_started","hook_name":"SessionStart"}`))
	if err != nil || !message.IsHook {
		t.Fatalf("message = %+v, err = %v", message, err)
	}
}
