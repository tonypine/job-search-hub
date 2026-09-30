package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrModelProviderNotFound = errors.New("model provider not found")
	ErrTaskRouteNotFound     = errors.New("no route for that task")
	// ErrModelProviderInUse refuses deleting a provider a route runs on.
	ErrModelProviderInUse = errors.New("a task runs on that provider; route it elsewhere first")
)

// RoutedTaskKinds are the kinds of background task that run on a routed
// model.
var RoutedTaskKinds = []string{AgentPromptKindJobFacts, AgentPromptKindMailTriage, AgentPromptKindLinkedInConversation}

// DefaultModelProviderName names the provider the hub creates from its
// settings, the local model server it used before routes existed.
const DefaultModelProviderName = "Local model"

// ModelProvider is a model server the hub can call, local or hosted. Its key
// is never served back; HasKey says whether one is set.
type ModelProvider struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	BaseURL        string    `json:"base_url"`
	APIKey         string    `json:"-"`
	HasKey         bool      `json:"has_key"`
	EnforcesSchema bool      `json:"enforces_schema"`
	CreatedAt      time.Time `json:"created_at"`
}

// ModelProviderInput adds or replaces a provider. A nil APIKey keeps the
// key already saved; an empty one removes it.
type ModelProviderInput struct {
	Name           string  `json:"name"`
	BaseURL        string  `json:"base_url"`
	APIKey         *string `json:"api_key,omitempty"`
	EnforcesSchema bool    `json:"enforces_schema"`
}

// TaskRoute is the provider and model a kind of task runs on, and its
// fallback when that provider is unreachable.
type TaskRoute struct {
	Kind               string     `json:"kind"`
	ProviderID         uuid.UUID  `json:"provider_id"`
	Model              string     `json:"model"`
	FallbackProviderID *uuid.UUID `json:"fallback_provider_id,omitempty"`
	FallbackModel      string     `json:"fallback_model,omitempty"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// TaskRouteInput replaces a route.
type TaskRouteInput struct {
	ProviderID         uuid.UUID  `json:"provider_id"`
	Model              string     `json:"model"`
	FallbackProviderID *uuid.UUID `json:"fallback_provider_id,omitempty"`
	FallbackModel      string     `json:"fallback_model,omitempty"`
}

const modelProviderColumns = `id, name, base_url, api_key, enforces_schema, created_at`

func scanModelProvider(row pgx.Row) (ModelProvider, error) {
	var provider ModelProvider
	err := row.Scan(&provider.ID, &provider.Name, &provider.BaseURL, &provider.APIKey, &provider.EnforcesSchema, &provider.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ModelProvider{}, ErrModelProviderNotFound
	}
	provider.HasKey = provider.APIKey != ""
	return provider, err
}

const taskRouteColumns = `kind, provider_id, model, fallback_provider_id, fallback_model, updated_at`

func scanTaskRoute(row pgx.Row) (TaskRoute, error) {
	var route TaskRoute
	err := row.Scan(&route.Kind, &route.ProviderID, &route.Model, &route.FallbackProviderID, &route.FallbackModel, &route.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TaskRoute{}, ErrTaskRouteNotFound
	}
	return route, err
}

func (s *Store) ListModelProviders(ctx context.Context) ([]ModelProvider, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+modelProviderColumns+` FROM model_providers ORDER BY created_at, name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ModelProvider, error) { return scanModelProvider(row) })
}

func (s *Store) GetModelProvider(ctx context.Context, id uuid.UUID) (ModelProvider, error) {
	return scanModelProvider(s.pool.QueryRow(ctx, `SELECT `+modelProviderColumns+` FROM model_providers WHERE id = $1`, id))
}

// SaveModelProvider adds a provider, or replaces the one with id.
func (s *Store) SaveModelProvider(ctx context.Context, actor Actor, id *uuid.UUID, input ModelProviderInput) (ModelProvider, error) {
	input.Name, input.BaseURL = strings.TrimSpace(input.Name), strings.TrimSuffix(strings.TrimSpace(input.BaseURL), "/")
	if input.Name == "" || !(strings.HasPrefix(input.BaseURL, "http://") || strings.HasPrefix(input.BaseURL, "https://")) {
		return ModelProvider{}, errors.New("a provider needs a name and an http(s) address, such as http://localhost:1234/v1")
	}
	var saved ModelProvider
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if id == nil {
			key := ""
			if input.APIKey != nil {
				key = strings.TrimSpace(*input.APIKey)
			}
			saved, err = scanModelProvider(tx.QueryRow(ctx, `
				INSERT INTO model_providers (name, base_url, api_key, enforces_schema) VALUES ($1, $2, $3, $4) RETURNING `+modelProviderColumns,
				input.Name, input.BaseURL, key, input.EnforcesSchema))
		} else {
			var key *string
			if input.APIKey != nil {
				trimmed := strings.TrimSpace(*input.APIKey)
				key = &trimmed
			}
			saved, err = scanModelProvider(tx.QueryRow(ctx, `
				UPDATE model_providers SET name = $2, base_url = $3, api_key = coalesce($4, api_key), enforces_schema = $5
				WHERE id = $1 RETURNING `+modelProviderColumns, *id, input.Name, input.BaseURL, key, input.EnforcesSchema))
		}
		if isUniqueViolation(err) {
			return errors.New("another provider has that name")
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "model_provider", entityID: saved.ID, operation: "save",
			after: map[string]any{"name": saved.Name, "base_url": saved.BaseURL, "has_key": saved.HasKey, "enforces_schema": saved.EnforcesSchema},
		})
	})
	return saved, err
}

// DeleteModelProvider removes a provider no route runs on.
func (s *Store) DeleteModelProvider(ctx context.Context, actor Actor, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		deleted, err := scanModelProvider(tx.QueryRow(ctx, `DELETE FROM model_providers WHERE id = $1 RETURNING `+modelProviderColumns, id))
		if isForeignKeyViolation(err) {
			return ErrModelProviderInUse
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "model_provider", entityID: deleted.ID, operation: "delete",
			before: map[string]string{"name": deleted.Name, "base_url": deleted.BaseURL}})
	})
}

func (s *Store) ListTaskRoutes(ctx context.Context) ([]TaskRoute, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+taskRouteColumns+` FROM task_routes ORDER BY kind`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TaskRoute, error) { return scanTaskRoute(row) })
}

func (s *Store) GetTaskRoute(ctx context.Context, kind string) (TaskRoute, error) {
	return scanTaskRoute(s.pool.QueryRow(ctx, `SELECT `+taskRouteColumns+` FROM task_routes WHERE kind = $1`, kind))
}

// SaveTaskRoute points a kind of task at a provider and model.
func (s *Store) SaveTaskRoute(ctx context.Context, actor Actor, kind string, input TaskRouteInput) (TaskRoute, error) {
	input.Model, input.FallbackModel = strings.TrimSpace(input.Model), strings.TrimSpace(input.FallbackModel)
	if !slices.Contains(RoutedTaskKinds, kind) {
		return TaskRoute{}, fmt.Errorf("%q isn't a task that runs on a routed model; those are %s", kind, strings.Join(RoutedTaskKinds, ", "))
	}
	if input.Model == "" {
		return TaskRoute{}, errors.New("a route needs a model")
	}
	if (input.FallbackProviderID == nil) != (input.FallbackModel == "") {
		return TaskRoute{}, errors.New("a fallback needs both a provider and a model")
	}
	var saved TaskRoute
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		saved, err = scanTaskRoute(tx.QueryRow(ctx, `
			INSERT INTO task_routes (kind, provider_id, model, fallback_provider_id, fallback_model) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (kind) DO UPDATE SET provider_id = EXCLUDED.provider_id, model = EXCLUDED.model,
				fallback_provider_id = EXCLUDED.fallback_provider_id, fallback_model = EXCLUDED.fallback_model, updated_at = now()
			RETURNING `+taskRouteColumns, kind, input.ProviderID, input.Model, input.FallbackProviderID, input.FallbackModel))
		if isForeignKeyViolation(err) {
			return ErrModelProviderNotFound
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "task_route", entityID: saved.ProviderID, operation: "route " + kind, after: input})
	})
	return saved, err
}

// EnsureDefaultTaskRoutes gives each kind that has no route one to the
// model server in the hub's settings, adding it as the "Local model"
// provider when it isn't one yet. It changes no route that exists.
func (s *Store) EnsureDefaultTaskRoutes(ctx context.Context, baseURL, model string, kinds []string) error {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || model == "" {
		return nil
	}
	system := Actor{Kind: ActorSystem}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		provider, err := scanModelProvider(tx.QueryRow(ctx, `SELECT `+modelProviderColumns+` FROM model_providers WHERE base_url = $1 ORDER BY created_at LIMIT 1`, baseURL))
		if errors.Is(err, ErrModelProviderNotFound) {
			provider, err = scanModelProvider(tx.QueryRow(ctx, `
				INSERT INTO model_providers (name, base_url) VALUES ($1, $2)
				ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name || ' (' || EXCLUDED.base_url || ')'
				RETURNING `+modelProviderColumns, DefaultModelProviderName, baseURL))
		}
		if err != nil {
			return err
		}
		for _, kind := range kinds {
			tag, err := tx.Exec(ctx, `INSERT INTO task_routes (kind, provider_id, model) VALUES ($1, $2, $3) ON CONFLICT (kind) DO NOTHING`, kind, provider.ID, model)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				if err := insertChange(ctx, tx, system, change{entityType: "task_route", entityID: provider.ID, operation: "route " + kind,
					after: map[string]string{"model": model, "base_url": baseURL}}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
