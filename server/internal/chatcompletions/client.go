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

	"github.com/google/uuid"
)

// ErrUnreachable means the model server did not answer at all, as when it is
// not running.
var ErrUnreachable = errors.New("the model server is unreachable")

// requestTimeout covers a model loading on its first request as well as a
// long answer.
const requestTimeout = 3 * time.Minute

type Client struct {
	// BaseURL is the API root, such as http://localhost:1234/v1.
	BaseURL    string
	HTTPClient *http.Client
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
}

// CompleteJSON returns the model's JSON object. Some models behind LM Studio,
// such as Qwen3.5, put a constrained answer in reasoning_content and leave
// content empty, so that is read when content is.
func (client *Client) CompleteJSON(ctx context.Context, request JSONRequest) (json.RawMessage, error) {
	startedAt := time.Now()
	answer, usage, outcome, err := client.complete(ctx, request)
	if client.RecordRun != nil {
		record := RunRecord{
			Kind: request.SchemaName, Task: request.Task, BaseURL: client.BaseURL, Model: request.Model,
			InputHash: hashInput(request), PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
			StartedAt: startedAt, Duration: time.Since(startedAt), Outcome: outcome,
		}
		if err != nil {
			record.Error = err.Error()
		} else {
			record.Output = answer
		}
		client.RecordRun(context.WithoutCancel(ctx), record)
	}
	return answer, err
}

type tokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// hashInput identifies what the model was asked, so runs on the same input
// can be compared.
func hashInput(request JSONRequest) string {
	sum := sha256.Sum256([]byte(request.System + "\x00" + request.User + "\x00" + string(request.Schema)))
	return hex.EncodeToString(sum[:])
}

func (client *Client) complete(ctx context.Context, request JSONRequest) (json.RawMessage, tokenUsage, string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       request.Model,
		"temperature": 0,
		"max_tokens":  request.MaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": request.System},
			{"role": "user", "content": request.User},
		},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": request.SchemaName, "strict": true, "schema": request.Schema},
		},
	})
	if err != nil {
		return nil, tokenUsage{}, RunFailed, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, tokenUsage{}, RunFailed, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")

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
		return nil, completion.Usage, RunInvalid, errors.New("the answer was cut off at the token limit")
	}
	answer := strings.TrimSpace(choice.Message.Content)
	if answer == "" {
		answer = strings.TrimSpace(choice.Message.ReasoningContent)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(answer), &object); err != nil {
		return nil, completion.Usage, RunInvalid, fmt.Errorf("the answer is not a JSON object: %w", err)
	}
	return json.RawMessage(answer), completion.Usage, RunSucceeded, nil
}
