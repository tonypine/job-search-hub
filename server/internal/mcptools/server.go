// Package mcptools serves the hub's data as MCP tools. For agents, these tools
// are the only way to write to the hub.
package mcptools

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

var schemaOptions = &jsonschema.ForOptions{
	TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[uuid.UUID](): {Type: "string", Format: "uuid"},
	},
}

// NewServer registers every hub tool against the store.
func NewServer(hub *store.Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "job-search-hub", Version: "0.1.0"}, nil)
	addCompanyTools(server, hub)
	addWatchListTools(server, hub)
	return server
}

// NewHandler serves the MCP server over streamable HTTP to callers holding a
// token the verifier accepts.
func NewHandler(server *mcp.Server, verifier auth.TokenVerifier) http.Handler {
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	requireToken := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})
	return requireToken(streamable)
}

// addTool registers a typed tool with schemas that describe a uuid.UUID as
// the string it marshals to, not the byte array it is in Go.
func addTool[In, Out any](server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	input, err := jsonschema.For[In](schemaOptions)
	if err != nil {
		panic(fmt.Sprintf("input schema for %s: %v", tool.Name, err))
	}
	output, err := jsonschema.For[Out](schemaOptions)
	if err != nil {
		panic(fmt.Sprintf("output schema for %s: %v", tool.Name, err))
	}
	tool.InputSchema = input
	tool.OutputSchema = output
	mcp.AddTool(server, tool, handler)
}
