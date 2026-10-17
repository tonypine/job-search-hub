// Package openrouter is what the hub knows of OpenRouter, an
// OpenAI-compatible service that serves many vendors' models behind one key:
// its address, the headers it attributes requests by, its catalogue of models
// and their prices, the cost it reports with each answer, and its errors.
package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
)

// BaseURL is OpenRouter's API root.
const BaseURL = "https://openrouter.ai/api/v1"

// The app attribution OpenRouter reads from each request, so the owner's
// usage is filed under the hub.
const (
	appURL   = "https://github.com/tonypine/job-search-hub"
	appTitle = "Job Search Hub"
)

// catalogueTimeout covers OpenRouter's whole catalogue, a few hundred models.
const catalogueTimeout = 15 * time.Second

// Configure makes client ask OpenRouter: with the hub's attribution, for each
// request's cost, and reading OpenRouter's errors.
func Configure(client *chatcompletions.Client) {
	client.Headers = getHeaders()
	client.AskForCost = true
	client.ReadError = ReadError
}

func getHeaders() map[string]string {
	return map[string]string{"HTTP-Referer": appURL, "X-Title": appTitle}
}

// ReadError reads one of OpenRouter's errors. Down, busy or out of credits
// reads as unreachable, so a route's fallback runs instead: a timeout (408),
// a rate limit (429), no credits left (402), or the model's provider down
// (502, 503 and the like).
func ReadError(status int, payload []byte) error {
	var envelope struct {
		Error struct {
			Message  string `json:"message"`
			Metadata struct {
				ProviderName string `json:"provider_name"`
			} `json:"metadata"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(payload))
	if json.Unmarshal(payload, &envelope) == nil && envelope.Error.Message != "" {
		message = envelope.Error.Message
		if envelope.Error.Metadata.ProviderName != "" {
			message += " (" + envelope.Error.Metadata.ProviderName + ")"
		}
	}
	switch {
	case status == http.StatusUnauthorized:
		return fmt.Errorf("OpenRouter refused the key: %s", message)
	case status == http.StatusPaymentRequired:
		return fmt.Errorf("%w: OpenRouter has no credits left: %s", chatcompletions.ErrUnreachable, message)
	case status == http.StatusForbidden:
		return fmt.Errorf("OpenRouter's moderation refused the input: %s", message)
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return fmt.Errorf("%w: OpenRouter is down or busy (%d): %s", chatcompletions.ErrUnreachable, status, message)
	default:
		return fmt.Errorf("OpenRouter answered %d: %s", status, message)
	}
}

// Model is a model in OpenRouter's catalogue, with its prices in US dollars
// per million tokens in and out, as decimals. A price is nil for a model
// priced by whichever model it routes to, such as openrouter/auto.
type Model struct {
	ID                  string
	Name                string
	InputUSDPerMillion  *string
	OutputUSDPerMillion *string
	ContextLength       int
}

// ListModels reads the catalogue at baseURL, sorted by id.
func ListModels(ctx context.Context, baseURL, key string) ([]Model, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	for name, value := range getHeaders() {
		request.Header.Set(name, value)
	}
	response, err := (&http.Client{Timeout: catalogueTimeout}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("OpenRouter doesn't answer: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, ReadError(response.StatusCode, payload)
	}
	var catalogue struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &catalogue); err != nil {
		return nil, fmt.Errorf("read OpenRouter's models: %w", err)
	}
	models := make([]Model, 0, len(catalogue.Data))
	for _, model := range catalogue.Data {
		models = append(models, Model{
			ID: model.ID, Name: model.Name, ContextLength: model.ContextLength,
			InputUSDPerMillion: convertToPerMillion(model.Pricing.Prompt), OutputUSDPerMillion: convertToPerMillion(model.Pricing.Completion),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// convertToPerMillion turns OpenRouter's price per token, a decimal such as
// "0.000003", into the price per million tokens, "3", exactly. A price that
// isn't a decimal, or is negative as a routed model's is, is nil.
func convertToPerMillion(perToken string) *string {
	price, ok := new(big.Rat).SetString(strings.TrimSpace(perToken))
	if !ok || price.Sign() < 0 {
		return nil
	}
	text := price.Mul(price, big.NewRat(1_000_000, 1)).FloatString(6)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	return &text
}
