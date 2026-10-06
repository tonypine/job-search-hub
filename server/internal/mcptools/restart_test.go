package mcptools_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/mcptools"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type pingOutput struct {
	Pong bool `json:"pong"`
}

// newPingServer is an MCP server with one tool, ping, which needs no database.
func newPingServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "job-search-hub", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, pingOutput, error) {
		return nil, pingOutput{Pong: true}, nil
	})
	return server
}

// restartableHub serves the MCP endpoint at one address across restarts: a
// restart swaps in a new handler that remembers nothing, as a new process.
type restartableHub struct {
	handler atomic.Pointer[http.Handler]
	url     string
}

func startRestartableHub(t *testing.T) *restartableHub {
	t.Helper()
	hub := &restartableHub{}
	hub.restart()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		(*hub.handler.Load()).ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	hub.url = server.URL
	return hub
}

func (hub *restartableHub) restart() {
	handler := mcptools.NewHandler(newPingServer(), newPingServer(), tokens.NewVerifier(ownerToken, nil))
	hub.handler.Store(&handler)
}

func TestAnOpenSessionsNextCallWorksAfterARestart(t *testing.T) {
	hub := startRestartableHub(t)
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   hub.url,
		HTTPClient: &http.Client{Transport: bearerTransport{token: ownerToken}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()
	if result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "ping"}); err != nil || result.IsError {
		t.Fatalf("ping before the restart: %+v, %v", result, err)
	}

	hub.restart()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "ping"})
	if err != nil || result.IsError {
		t.Fatalf("ping after the restart: %+v, %v", result, err)
	}
}

func TestACallWithASessionIDFromBeforeARestartIsServed(t *testing.T) {
	hub := startRestartableHub(t)
	request, _ := http.NewRequest(http.MethodPost, hub.url, strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"ping","arguments":{}}}`))
	request.Header.Set("Authorization", "Bearer "+ownerToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Session-Id", "a-session-the-old-process-gave")
	request.Header.Set("Mcp-Protocol-Version", "2025-06-18")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"pong":true`) {
		t.Fatalf("call with a stale session: %d %s", response.StatusCode, body)
	}
}
