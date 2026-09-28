package mcptools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/mcptools"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

var ownerToken = strings.Repeat("o", 64)

type hubUnderTest struct {
	pool  *pgxpool.Pool
	store *store.Store
	url   string
}

func startHub(t *testing.T) hubUnderTest {
	t.Helper()
	pool := testdatabase.New(t)
	hub := store.New(pool)
	handler := mcptools.NewHandler(mcptools.NewServer(hub, stubJobBoards{}), tokens.NewVerifier(ownerToken, hub))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return hubUnderTest{pool: pool, store: hub, url: server.URL}
}

// startAgentRun records a running agent run whose token expires at
// expiresAt, and returns the run with its token.
func startAgentRun(t *testing.T, hub hubUnderTest, expiresAt time.Time) (store.AgentRun, string) {
	t.Helper()
	token, tokenHash := tokens.NewAgentRunToken()
	run, err := hub.store.StartAgentRun(context.Background(), store.AgentRunKindCompanyTriage, "stripe.com", tokenHash, expiresAt)
	if err != nil {
		t.Fatalf("start agent run: %v", err)
	}
	return run, token
}

// stubJobBoards knows one board, "acme", on each provider it can verify.
type stubJobBoards struct{}

func (stubJobBoards) Verify(_ context.Context, provider, boardToken string) (jobboards.Verification, error) {
	if !slices.Contains([]string{jobboards.Greenhouse, jobboards.Lever, jobboards.Ashby}, provider) {
		return jobboards.Verification{}, jobboards.ErrUnsupportedProvider
	}
	if boardToken != "acme" {
		return jobboards.Verification{}, nil
	}
	return jobboards.Verification{Verified: true, OpenPostingCount: 5, BoardURL: "https://boards.example/" + provider + "/acme"}, nil
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
