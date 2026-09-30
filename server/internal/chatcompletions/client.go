// Package chatcompletions asks a model for JSON through the OpenAI-compatible
// chat-completions API that local servers such as LM Studio serve.
package chatcompletions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
)

// ErrUnreachable means the model server did not answer at all, as when it is
// not running.
var ErrUnreachable = errors.New("the model server is unreachable")

// ErrInvalidAnswer means the server answered, but not with the JSON object
// asked for: cut off, not JSON, or not matching the schema.
var ErrInvalidAnswer = errors.New("the model's answer is invalid")

// requestTimeout covers a model loading on its first request as well as a
// long answer.
const requestTimeout = 3 * time.Minute

type Client struct {
	// BaseURL is the API root, such as http://localhost:1234/v1.
	BaseURL    string
	HTTPClient *http.Client
	// APIKey, when set, is sent as a bearer token, as hosted providers need.
	APIKey string
	// RecordRun, when set, hears about every request once it ends, whatever
	// its outcome, for the hub's record of task runs.
	RecordRun func(context.Context, RunRecord)
}

// The outcomes of a request.
const (
	RunSucceeded = "succeeded"
	// RunFailed is a request that got no usable answer: the server was
	// unreachable or answered with an error.
	RunFailed = "failed"
	// RunInvalid is an answer that isn't the JSON object asked for, or was
	// cut off at the token limit.
	RunInvalid = "invalid"
)

// RunRecord is one request as the record of task runs keeps it.
type RunRecord struct {
	// Kind is the request's schema name, which is its prompt's kind.
	Kind             string
	Task             TaskLabel
	BaseURL          string
	Model            string
	InputHash        string
	Output           json.RawMessage
	PromptTokens     int
	CompletionTokens int
	StartedAt        time.Time
	Duration         time.Duration
	Outcome          string
	Error            string
}

// TaskLabel says what a request is for: the record keeps it; the model never
// sees it.
type TaskLabel struct {
	// SubjectID is the job, mail or conversation the request is about.
	SubjectID     *uuid.UUID
	PromptID      *uuid.UUID
	PromptVersion int
}

func NewClient(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimSuffix(baseURL, "/"), HTTPClient: &http.Client{Timeout: requestTimeout}}
}

// JSONRequest asks Model to answer User, following System, with a JSON object
// that matches Schema.
type JSONRequest struct {
	Model      string
	System     string
	User       string
	SchemaName string
	Schema     json.RawMessage
	MaxTokens  int
	Task       TaskLabel
	// Examples are worked examples sent before User as earlier turns of the
	// conversation, each an input and the answer the model should give.
	Examples []Example
	// SchemaNotEnforced is for a server that can't enforce a schema: it is
	// only asked for a JSON object, and the answer is validated against the
	// schema instead.
	SchemaNotEnforced bool
}

// Example is a worked example: an input and the answer for it.
type Example struct {
	Input  string          `json:"input"`
	Answer json.RawMessage `json:"answer"`
}

// Answer is a model's JSON object and the model that gave it.
type Answer struct {
	Object json.RawMessage
	Model  string
}

// CompleteJSON returns the model's JSON object. Some models behind LM Studio,
// such as Qwen3.5, put a constrained answer in reasoning_content and leave
// content empty, so that is read when content is.
func (client *Client) CompleteJSON(ctx context.Context, request JSONRequest) (Answer, error) {
	startedAt := time.Now()
	answer, usage, outcome, err := client.complete(ctx, request)
	if client.RecordRun != nil {
		record := RunRecord{
			Kind: request.SchemaName, Task: request.Task, BaseURL: client.BaseURL, Model: request.Model,
			InputHash: HashInput(request), PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
			StartedAt: startedAt, Duration: time.Since(startedAt), Outcome: outcome,
		}
		if err != nil {
			record.Error = err.Error()
		} else {
			record.Output = answer
		}
		client.RecordRun(context.WithoutCancel(ctx), record)
	}
	if err != nil {
		return Answer{}, err
	}
	return Answer{Object: answer, Model: request.Model}, nil
}

type tokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// HashInput identifies what the model was asked, so runs on the same input
// can be compared.
func HashInput(request JSONRequest) string {
	text := request.System + "\x00" + request.User + "\x00" + string(request.Schema)
	for _, example := range request.Examples {
		text += "\x00" + example.Input + "\x00" + string(example.Answer)
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (client *Client) complete(ctx context.Context, request JSONRequest) (json.RawMessage, tokenUsage, string, error) {
	// response_format only constrains decoding: servers such as LM Studio and
	// llama.cpp never show the schema to the model, which then guesses each
	// field's meaning from its name.
	system := strings.TrimRight(request.System, "\n") + "\n\nAnswer with only a JSON object that matches this JSON Schema:\n" + string(request.Schema)
	responseFormat := map[string]any{
		"type":        "json_schema",
		"json_schema": map[string]any{"name": request.SchemaName, "strict": true, "schema": request.Schema},
	}
	if request.SchemaNotEnforced {
		responseFormat = map[string]any{"type": "json_object"}
	}
	body, err := json.Marshal(map[string]any{
		"model":           request.Model,
		"temperature":     0,
		"max_tokens":      request.MaxTokens,
		"messages":        buildMessages(system, request),
		"response_format": responseFormat,
	})
	if err != nil {
		return nil, tokenUsage{}, RunFailed, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, tokenUsage{}, RunFailed, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if client.APIKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+client.APIKey)
	}

	response, err := client.HTTPClient.Do(httpRequest)
	if err != nil {
		return nil, tokenUsage{}, RunFailed, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, tokenUsage{}, RunFailed, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, tokenUsage{}, RunFailed, fmt.Errorf("the model server answered %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}

	var completion struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Usage tokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(payload, &completion); err != nil {
		return nil, tokenUsage{}, RunFailed, fmt.Errorf("read the completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, completion.Usage, RunFailed, errors.New("the completion has no choices")
	}
	choice := completion.Choices[0]
	if choice.FinishReason == "length" {
		return nil, completion.Usage, RunInvalid, fmt.Errorf("%w: it was cut off at the token limit", ErrInvalidAnswer)
	}
	answer := strings.TrimSpace(choice.Message.Content)
	if answer == "" {
		answer = strings.TrimSpace(choice.Message.ReasoningContent)
	}
	answer = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(answer), "```json"), "```")
	var object map[string]any
	if err := json.Unmarshal([]byte(answer), &object); err != nil {
		return nil, completion.Usage, RunInvalid, fmt.Errorf("%w: it is not a JSON object: %v", ErrInvalidAnswer, err)
	}
	if request.SchemaNotEnforced {
		if err := validateAgainstSchema(object, request.Schema); err != nil {
			return nil, completion.Usage, RunInvalid, fmt.Errorf("%w: %v", ErrInvalidAnswer, err)
		}
	}
	return json.RawMessage(strings.TrimSpace(answer)), completion.Usage, RunSucceeded, nil
}

// buildMessages lays out the conversation: the system prompt, each worked
// example as a user turn and the assistant's answer, then the input.
func buildMessages(system string, request JSONRequest) []map[string]string {
	messages := []map[string]string{{"role": "system", "content": system}}
	for _, example := range request.Examples {
		messages = append(messages,
			map[string]string{"role": "user", "content": example.Input},
			map[string]string{"role": "assistant", "content": string(example.Answer)})
	}
	return append(messages, map[string]string{"role": "user", "content": request.User})
}

// validateAgainstSchema checks an answer a server wasn't made to fit.
func validateAgainstSchema(answer map[string]any, schemaText json.RawMessage) error {
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaText, &schema); err != nil {
		return fmt.Errorf("read the schema: %w", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve the schema: %w", err)
	}
	if err := resolved.Validate(answer); err != nil {
		return fmt.Errorf("the answer doesn't match the schema: %w", err)
	}
	return nil
}
