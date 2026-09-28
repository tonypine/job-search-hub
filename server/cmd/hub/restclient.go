package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(config.HubURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+config.OwnerToken)
	httpRequest.Header.Set("Content-Type", "application/json")

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
		return fmt.Errorf("POST %s: %d %s", path, httpResponse.StatusCode, failure.Error)
	}
	return json.NewDecoder(httpResponse.Body).Decode(response)
}
