package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct {
	token string
}

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	authorized := request.Clone(request.Context())
	authorized.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(authorized)
}

// connectToHub opens an MCP session with the hub as the owner.
func connectToHub(ctx context.Context, config cliConfig) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "hub-cli", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             strings.TrimSuffix(config.HubURL, "/") + "/mcp",
		HTTPClient:           &http.Client{Transport: bearerTransport{token: config.OwnerToken}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to the hub at %s: %w", config.HubURL, err)
	}
	return session, nil
}

// callTool calls a hub tool and decodes its structured result into Out. A
// tool error comes back as a Go error carrying the tool's message.
func callTool[Out any](ctx context.Context, session *mcp.ClientSession, name string, arguments any) (Out, error) {
	var output Out
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return output, fmt.Errorf("%s: %w", name, err)
	}
	if result.IsError {
		var message strings.Builder
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				message.WriteString(text.Text)
			}
		}
		return output, errors.New(message.String())
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return output, err
	}
	err = json.Unmarshal(encoded, &output)
	return output, err
}
