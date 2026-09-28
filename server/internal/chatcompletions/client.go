// Package chatcompletions asks a model for JSON through the OpenAI-compatible
// chat-completions API that local servers such as LM Studio serve.
package chatcompletions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
}

// CompleteJSON returns the model's JSON object. Some models behind LM Studio,
// such as Qwen3.5, put a constrained answer in reasoning_content and leave
// content empty, so that is read when content is.
func (client *Client) CompleteJSON(ctx context.Context, request JSONRequest) (json.RawMessage, error) {
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
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := client.HTTPClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the model server answered %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}

	var completion struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &completion); err != nil {
		return nil, fmt.Errorf("read the completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("the completion has no choices")
	}
	choice := completion.Choices[0]
	if choice.FinishReason == "length" {
		return nil, errors.New("the answer was cut off at the token limit")
	}
	answer := strings.TrimSpace(choice.Message.Content)
	if answer == "" {
		answer = strings.TrimSpace(choice.Message.ReasoningContent)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(answer), &object); err != nil {
		return nil, fmt.Errorf("the answer is not a JSON object: %w", err)
	}
	return json.RawMessage(answer), nil
}
