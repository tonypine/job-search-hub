package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/boardpoller"
	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/mcptools"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

var testOwnerToken = strings.Repeat("o", 64)

type hubUnderTest struct {
	pool   *pgxpool.Pool
	store  *store.Store
	config cliConfig
}

// startHub serves the hub's REST routes and MCP endpoint in-process, the way
// hub-server wires them.
func startHub(t *testing.T) hubUnderTest {
	t.Helper()
	pool := testdatabase.New(t)
	hub := store.New(pool)
	verifier := tokens.NewVerifier(testOwnerToken, hub)
	routes := http.NewServeMux()
	api.RegisterAgentRunRoutes(routes, hub, auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	}))
	boards := jobboards.NewVerifier()
	routes.Handle("/mcp", mcptools.NewHandler(mcptools.NewServer(hub, boards, boardpoller.New(hub, boards)), verifier))
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return hubUnderTest{pool: pool, store: hub, config: cliConfig{HubURL: server.URL, OwnerToken: testOwnerToken}}
}

// installFakeClaude points HUB_CLAUDE_BIN at a script that records its
// arguments and replays streamLines as a session's stream-json output.
func installFakeClaude(t *testing.T, streamLines ...string) (argumentsPath string) {
	t.Helper()
	directory := t.TempDir()
	streamPath := filepath.Join(directory, "stream.jsonl")
	argumentsPath = filepath.Join(directory, "arguments.txt")
	scriptPath := filepath.Join(directory, "claude")
	if err := os.WriteFile(streamPath, []byte(strings.Join(streamLines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argumentsPath + "\ncat " + streamPath + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUB_CLAUDE_BIN", scriptPath)
	t.Setenv("HOME", t.TempDir())
	return argumentsPath
}

func initLine(extraPlugin string) string {
	plugins := []map[string]string{{"name": "telemetry", "source": "telemetry@builtin"}}
	if extraPlugin != "" {
		plugins = append(plugins, map[string]string{"name": extraPlugin, "source": extraPlugin + "@marketplace"})
	}
	encoded, _ := json.Marshal(map[string]any{
		"type": "system", "subtype": "init", "session_id": "session-1",
		"plugins": plugins, "skills": []string{}, "slash_commands": []string{},
		"mcp_servers": []map[string]string{{"name": "hub", "status": "connected"}},
		"tools":       []string{"StructuredOutput", "WebFetch", "WebSearch", "mcp__hub__create_company"},
	})
	return string(encoded)
}

func resultLine(companyID string) string {
	encoded, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "session_id": "session-1", "total_cost_usd": 0.42,
		"structured_output": map[string]any{
			"company_id": companyID, "domain": "acme.com", "job_board": nil, "people_added": 0,
			"unresolved": []string{"no job board link on the careers page"}, "summary": "Stored Acme.",
		},
	})
	return string(encoded)
}

const toolUseLine = `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__hub__find_companies","input":{"query":"acme.com"}}]}}`

func readAgentRun(t *testing.T, pool *pgxpool.Pool) (status, sessionID, cost, errorText string) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT status, claude_session_id, COALESCE(cost_usd_estimate::text, ''), error FROM agent_runs`).Scan(&status, &sessionID, &cost, &errorText)
	if err != nil {
		t.Fatalf("read the agent run: %v", err)
	}
	return status, sessionID, cost, errorText
}

func TestAddCompanyRecordsTheRunAndWatchesTheCompany(t *testing.T) {
	hub := startHub(t)
	company, _, err := hub.store.CreateCompany(context.Background(), store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	argumentsPath := installFakeClaude(t, initLine(""), toolUseLine, resultLine(company.ID.String()))

	var out bytes.Buffer
	if err := addCompany(context.Background(), hub.config, "acme.com", triageOptions{foundVia: "Referral: a former colleague"}, &out); err != nil {
		t.Fatalf("add company: %v\n%s", err, out.String())
	}

	status, sessionID, cost, _ := readAgentRun(t, hub.pool)
	if status != store.AgentRunSucceeded || sessionID != "session-1" || cost != "0.42" {
		t.Fatalf("run: status=%s session=%s cost=%s", status, sessionID, cost)
	}
	if since, err := hub.store.GetWatchedSince(context.Background(), company.ID); err != nil || since == nil {
		t.Fatalf("the company is not watched: %v", err)
	}
	for _, want := range []string{"· find_companies", "Stored Acme.", "unresolved: no job board link", "Acme (acme.com)", "Found via: Referral: a former colleague", "On the watch list since"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}

	recorded, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	arguments := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	for _, flag := range []string{"--strict-mcp-config", "--disable-slash-commands"} {
		if !slices.Contains(arguments, flag) {
			t.Errorf("claude was started without %s", flag)
		}
	}
	mcpConfigIndex := slices.Index(arguments, "--mcp-config")
	if mcpConfigIndex < 0 {
		t.Fatal("claude was started without --mcp-config")
	}
	if _, err := os.Stat(arguments[mcpConfigIndex+1]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the MCP config with the run's token was left behind: %v", err)
	}
}

func TestAnUnisolatedSessionFailsTheRunAndWatchesNothing(t *testing.T) {
	hub := startHub(t)
	company, _, _ := hub.store.CreateCompany(context.Background(), store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	installFakeClaude(t, initLine("superpowers"), toolUseLine, resultLine(company.ID.String()))

	var out bytes.Buffer
	err := addCompany(context.Background(), hub.config, "acme.com", triageOptions{}, &out)
	if err == nil || !strings.Contains(err.Error(), "not isolated") {
		t.Fatalf("err = %v, want an isolation failure", err)
	}

	status, _, _, errorText := readAgentRun(t, hub.pool)
	if status != store.AgentRunFailed || !strings.Contains(errorText, "superpowers@marketplace") {
		t.Fatalf("run: status=%s error=%q", status, errorText)
	}
	if since, _ := hub.store.GetWatchedSince(context.Background(), company.ID); since != nil {
		t.Fatal("the company was watched after a failed run")
	}
}
