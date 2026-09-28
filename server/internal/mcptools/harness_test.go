package mcptools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/mcptools"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

var ownerToken = strings.Repeat("o", 64)

type hubUnderTest struct {
	pool *pgxpool.Pool
	url  string
}

func startHub(t *testing.T) hubUnderTest {
	t.Helper()
	pool := testdatabase.New(t)
	handler := mcptools.NewHandler(mcptools.NewServer(store.New(pool)), tokens.NewVerifier(ownerToken))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return hubUnderTest{pool: pool, url: server.URL}
}

type bearerTransport struct{ token string }

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	authorized := request.Clone(request.Context())
	authorized.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(authorized)
}

func connect(t *testing.T, hub hubUnderTest, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   hub.url,
		HTTPClient: &http.Client{Transport: bearerTransport{token: token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// callTool calls a tool that is expected to succeed and decodes its
// structured result into Out.
func callTool[Out any](t *testing.T, session *mcp.ClientSession, name string, arguments any) Out {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned a tool error: %s", name, resultText(result))
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("%s: encode structured content: %v", name, err)
	}
	var output Out
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("%s: decode structured content: %v", name, err)
	}
	return output
}

// callFailingTool calls a tool that is expected to return a tool error, and
// returns the error's text.
func callFailingTool(t *testing.T, session *mcp.ClientSession, name string, arguments any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !result.IsError {
		t.Fatalf("%s succeeded, want a tool error", name)
	}
	return resultText(result)
}

func resultText(result *mcp.CallToolResult) string {
	var text strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			text.WriteString(textContent.Text)
		}
	}
	return text.String()
}
