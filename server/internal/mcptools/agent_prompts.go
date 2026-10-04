package mcptools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// The kind and body fields are described in agentPromptInputSchema, from the
// hub's own list of prompts.

type getAgentPromptInput struct {
	Kind    string `json:"kind"`
	Version *int   `json:"version,omitempty" jsonschema:"a past version to read; the active, latest version when absent"`
}

type updateAgentPromptInput struct {
	Kind         string               `json:"kind"`
	Body         string               `json:"body"`
	ResultSchema json.RawMessage      `json:"result_schema,omitempty" jsonschema:"the JSON schema object the answer must match, kept as written: the model fills the fields in the order listed; for job_facts, one property per fact, each with a title and a description; the previous version's schema is kept when absent"`
	Examples     []promptExampleInput `json:"examples,omitempty" jsonschema:"worked examples shown to the model before the real input, each an input and the answer object it should give; the previous version's examples are kept when absent, and an empty list removes them"`
	Note         string               `json:"note,omitempty" jsonschema:"why this version exists, e.g. what it changes"`
}

type promptExampleInput struct {
	Input  string         `json:"input" jsonschema:"the input exactly as the model will see it, e.g. a posting's text"`
	Answer map[string]any `json:"answer" jsonschema:"the answer object for that input, matching the result schema"`
}

func addAgentPromptTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name:        "get_agent_prompt",
		Description: "Read a prompt, with the schema its answer must match: the active version, or a past one by number.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: agentPromptInputSchema[getAgentPromptInput]("get_agent_prompt"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getAgentPromptInput) (*mcp.CallToolResult, store.AgentPrompt, error) {
		if input.Version != nil {
			prompt, err := hub.GetAgentPrompt(ctx, input.Kind, *input.Version)
			return nil, prompt, err
		}
		prompt, err := hub.GetLatestAgentPrompt(ctx, input.Kind)
		return nil, prompt, err
	})

	addTool(server, &mcp.Tool{
		Name: "update_agent_prompt",
		Description: "Save a new version of a prompt, optionally with a new answer schema. It becomes active for the next run; " +
			"earlier versions stay readable. It can carry worked examples, sent to the model as earlier turns. " +
			"Saving a new job_facts version makes the hub read every open job's facts again. Owner only.",
		InputSchema: agentPromptInputSchema[updateAgentPromptInput]("update_agent_prompt"),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input updateAgentPromptInput) (*mcp.CallToolResult, store.AgentPrompt, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.AgentPrompt{}, err
		}
		newPrompt := store.NewAgentPrompt{Kind: input.Kind, Body: input.Body, Note: input.Note}
		if input.Examples != nil {
			newPrompt.Examples = []chatcompletions.Example{}
			for _, example := range input.Examples {
				answer, err := json.Marshal(example.Answer)
				if err != nil {
					return nil, store.AgentPrompt{}, err
				}
				newPrompt.Examples = append(newPrompt.Examples, chatcompletions.Example{Input: example.Input, Answer: answer})
			}
		}
		if schema := bytes.TrimSpace(readWrittenResultSchema(request, input.ResultSchema)); len(schema) > 0 && !bytes.Equal(schema, []byte("null")) {
			if schema[0] != '{' || !json.Valid(schema) {
				return nil, store.AgentPrompt{}, errors.New("result_schema must be a JSON object")
			}
			newPrompt.ResultSchema = schema
		}
		prompt, err := hub.SaveAgentPrompt(ctx, actor, newPrompt)
		return nil, prompt, err
	})
}

// agentPromptInputSchema is the named prompt tool's input schema, its kind
// listing every prompt the hub keeps and its body, when it has one, the
// placeholders the hub fills in each.
func agentPromptInputSchema[In any](name string) *jsonschema.Schema {
	schema := inputSchema[In](name)
	schema.Properties["kind"].Description = "the prompt: " + joinWith(store.AgentPromptKinds, " or ")
	if body := schema.Properties["body"]; body != nil {
		body.Description = "the whole new prompt; the hub fills in its placeholders at each run: " + describePlaceholders()
	}
	return schema
}

// describePlaceholders names each kind's placeholders, and the kinds that
// have none.
func describePlaceholders() string {
	placeholders := map[string][]string{}
	for _, info := range prompts.Kinds {
		placeholders[info.Kind] = info.Placeholders
	}
	var filled, plain []string
	for _, kind := range store.AgentPromptKinds {
		if len(placeholders[kind]) == 0 {
			plain = append(plain, kind)
			continue
		}
		filled = append(filled, fmt.Sprintf("in %s, %s", kind, joinWith(placeholders[kind], " and ")))
	}
	description := strings.Join(filled, "; ")
	if len(plain) > 0 {
		description += "; " + joinWith(plain, " and ") + " have none"
	}
	return description
}

// joinWith lists words as "a, b and c", last joining the final two.
func joinWith(words []string, last string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + last + words[len(words)-1]
}

// readWrittenResultSchema returns result_schema as the caller wrote it. The
// SDK hands the handler its arguments after a round trip through a map,
// which sorts the schema's keys, so it's read again from the request's raw
// arguments, falling back to the decoded one.
func readWrittenResultSchema(request *mcp.CallToolRequest, decoded json.RawMessage) json.RawMessage {
	if request == nil || request.Params == nil || len(request.Params.Arguments) == 0 {
		return decoded
	}
	var written struct {
		ResultSchema json.RawMessage `json:"result_schema"`
	}
	if json.Unmarshal(request.Params.Arguments, &written) != nil || len(written.ResultSchema) == 0 {
		return decoded
	}
	return written.ResultSchema
}
