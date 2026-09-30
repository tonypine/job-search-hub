package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type modelProvidersResponse struct {
	Providers []store.ModelProvider `json:"providers"`
}

type taskRoutesResponse struct {
	Routes []store.TaskRoute `json:"routes"`
}

type providerModelsResponse struct {
	Models []string `json:"models"`
}

// RegisterModelRoutingRoutes adds the owner-only routes for the model servers
// the hub can call and the model each kind of task runs on. A provider's key
// can be set but is never served back. modelsDir holds the GGUF files a hub
// runtime provider can run.
func RegisterModelRoutingRoutes(routes *http.ServeMux, hub *store.Store, modelsDir string, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("GET /v1/model-providers", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providers, err := hub.ListModelProviders(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, modelProvidersResponse{Providers: providers})
	})))

	saveProvider := func(w http.ResponseWriter, r *http.Request, id *uuid.UUID, successStatus int) {
		var input store.ModelProviderInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		saved, err := hub.SaveModelProvider(r.Context(), owner, id, input)
		switch {
		case errors.Is(err, store.ErrModelProviderNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, successStatus, saved)
		}
	}
	routes.Handle("POST /v1/model-providers", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saveProvider(w, r, nil, http.StatusCreated)
	})))
	routes.Handle("PUT /v1/model-providers/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		saveProvider(w, r, &id, http.StatusOK)
	})))
	routes.Handle("DELETE /v1/model-providers/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		err = hub.DeleteModelProvider(r.Context(), owner, id)
		switch {
		case errors.Is(err, store.ErrModelProviderNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case errors.Is(err, store.ErrModelProviderInUse):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})))

	routes.Handle("GET /v1/model-providers/{id}/models", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		provider, err := hub.GetModelProvider(r.Context(), id)
		if errors.Is(err, store.ErrModelProviderNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		var models []string
		if provider.Kind == store.ModelProviderKindHubRuntime {
			models, err = listModelFiles(modelsDir)
		} else {
			models, err = listServedModels(r.Context(), provider)
		}
		if err != nil {
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, providerModelsResponse{Models: models})
	})))

	routes.Handle("GET /v1/task-routes", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		taskRoutes, err := hub.ListTaskRoutes(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, taskRoutesResponse{Routes: taskRoutes})
	})))
	routes.Handle("PUT /v1/task-routes/{kind}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input store.TaskRouteInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		saved, err := hub.SaveTaskRoute(r.Context(), owner, r.PathValue("kind"), input)
		switch {
		case errors.Is(err, store.ErrModelProviderNotFound):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "no such provider"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, saved)
		}
	})))
}

// listModelFiles names the GGUF files in the models folder.
func listModelFiles(modelsDir string) ([]string, error) {
	entries, err := os.ReadDir(modelsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	models := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".gguf") {
			models = append(models, entry.Name())
		}
	}
	return models, nil
}

// listServedModels asks an OpenAI-compatible provider which models it serves.
func listServedModels(ctx context.Context, provider store.ModelProvider) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if provider.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s doesn't answer: %w", provider.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %d to a request for its models", provider.Name, response.StatusCode)
	}
	var served struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&served); err != nil {
		return nil, fmt.Errorf("read %s's models: %w", provider.Name, err)
	}
	models := []string{}
	for _, model := range served.Data {
		models = append(models, model.ID)
	}
	sort.Strings(models)
	return models, nil
}
