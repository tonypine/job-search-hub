package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// postJSON sends request to the hub's REST API as the owner and decodes a
// successful answer into response.
func postJSON(ctx context.Context, config cliConfig, path string, request, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return callREST(ctx, config, http.MethodPost, path, bytes.NewReader(body), response)
}

// getJSON reads the hub's REST API as the owner.
func getJSON(ctx context.Context, config cliConfig, path string, response any) error {
	return callREST(ctx, config, http.MethodGet, path, nil, response)
}

func callREST(ctx context.Context, config cliConfig, method, path string, body io.Reader, response any) error {
	httpRequest, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(config.HubURL, "/")+path, body)
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+config.OwnerToken)
	if body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}

	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("reach the hub at %s: %w", config.HubURL, err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode >= http.StatusMultipleChoices {
		var failure struct {
			Error string `json:"error"`
		}
		json.NewDecoder(httpResponse.Body).Decode(&failure)
		return fmt.Errorf("%s %s: %d %s", method, path, httpResponse.StatusCode, failure.Error)
	}
	return json.NewDecoder(httpResponse.Body).Decode(response)
}
