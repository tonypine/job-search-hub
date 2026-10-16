// Package modelrouter sends each model request to the provider and model its
// kind of run is routed to, with the route's fallback when that provider is
// unreachable: an OpenAI-compatible server, OpenRouter, the hub's own runtime
// or the owner's Claude subscription, through the Claude CLI.
package modelrouter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/claudeprint"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/openrouter"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// HubRuntimeAddress stands for the hub's own runtime in the record of a
// request that never reached it.
const HubRuntimeAddress = "hub runtime"

// runtime runs the hub's own local models (see modelruntime.Runtime).
type runtime interface {
	Acquire(ctx context.Context, modelFile string) (baseURL string, release func(), err error)
}

// queue takes model calls one at a time (see modelqueue.Queue).
type queue interface {
	Wait(ctx context.Context, ticket modelqueue.Ticket) (release func(), err error)
}

// Router answers requests on the routed model. Routes are read on every
// request, so a change applies to the next one.
type Router struct {
	hub *store.Store
	// Queue, when set, gives each request its turn: by the priority in its
	// context, else its kind's. Claude's runs don't queue: they take no turn
	// on this Mac's models, as before they were routed.
	Queue queue
	// Runtime serves routes to a hub runtime provider; nil leaves them
	// unreachable.
	Runtime runtime
	// Claude is the CLI routes to the Claude subscription run on, each with
	// its route's model and the router's RecordRun; nil, as when the hub
	// found no CLI, leaves them unreachable.
	Claude *claudeprint.Client
	// RecordRun, when set, hears about every attempt, fallbacks and retries
	// included.
	RecordRun func(context.Context, chatcompletions.RunRecord)
}

func New(hub *store.Store) *Router {
	return &Router{hub: hub}
}

// CompleteJSON answers request on the model its run kind is routed to: its
// schema name, or the kind its task names, as a full brief's. A provider that
// can't enforce a schema gets one retry for an invalid answer. When the
// route's provider is unreachable, the fallback answers, and its run names
// the provider it stood in for.
func (router *Router) CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	kind := request.GetRunKind()
	route, err := router.hub.GetTaskRoute(ctx, kind)
	if errors.Is(err, store.ErrTaskRouteNotFound) {
		return chatcompletions.Answer{}, fmt.Errorf("no model is routed for %s: %w", kind, err)
	}
	if err != nil {
		return chatcompletions.Answer{}, err
	}
	answer, err := router.completeOnProvider(ctx, route.ProviderID, route.Model, request, router.RecordRun)
	if !errors.Is(err, chatcompletions.ErrUnreachable) || route.FallbackProviderID == nil {
		return answer, err
	}
	provider, readErr := router.hub.GetModelProvider(ctx, route.ProviderID)
	if readErr != nil {
		return chatcompletions.Answer{}, readErr
	}
	recordRun := router.RecordRun
	if recordRun != nil {
		recordRun = func(ctx context.Context, record chatcompletions.RunRecord) {
			record.FallbackFor = provider.Name
			router.RecordRun(ctx, record)
		}
	}
	return router.completeOnProvider(ctx, *route.FallbackProviderID, route.FallbackModel, request, recordRun)
}

// CompleteJSONOn asks a given provider and model, past the task's route, as a
// comparison does. It still waits its turn in the model queue.
func (router *Router) CompleteJSONOn(ctx context.Context, providerID uuid.UUID, model string, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	return router.completeOnProvider(ctx, providerID, model, request, router.RecordRun)
}

// waitInQueue waits for the request's turn at the model; without a queue it
// goes at once.
func (router *Router) waitInQueue(ctx context.Context, model string, request chatcompletions.JSONRequest) (func(), error) {
	if router.Queue == nil {
		return func() {}, nil
	}
	kind := request.GetRunKind()
	return router.Queue.Wait(ctx, modelqueue.Ticket{
		Kind: kind, SubjectID: request.Task.SubjectID, Model: model,
		Priority: modelqueue.GetPriority(ctx, getKindPriority(kind)),
	})
}

// getKindPriority is how soon a kind of task runs when nobody asked for it:
// sorting shows up in the owner's inbox views, facts can wait.
func getKindPriority(kind string) modelqueue.Priority {
	if kind == store.AgentPromptKindMailTriage || kind == store.AgentPromptKindLinkedInConversation {
		return modelqueue.PrioritySorting
	}
	return modelqueue.PriorityBackground
}

// completeOnProvider asks one provider, once its turn in the queue comes,
// telling recordRun about each attempt.
func (router *Router) completeOnProvider(ctx context.Context, providerID uuid.UUID, model string, request chatcompletions.JSONRequest,
	recordRun func(context.Context, chatcompletions.RunRecord)) (chatcompletions.Answer, error) {
	provider, err := router.hub.GetModelProvider(ctx, providerID)
	if err != nil {
		return chatcompletions.Answer{}, fmt.Errorf("read the model provider: %w", err)
	}
	if provider.Kind == store.ModelProviderKindClaudeSubscription {
		return router.completeOnClaude(ctx, provider, model, request, recordRun)
	}
	release, err := router.waitInQueue(ctx, model, request)
	if err != nil {
		return chatcompletions.Answer{}, err
	}
	defer release()
	baseURL := provider.BaseURL
	if provider.Kind == store.ModelProviderKindHubRuntime {
		if router.Runtime == nil {
			return chatcompletions.Answer{}, fmt.Errorf("%w: the hub's model runtime isn't set up", chatcompletions.ErrUnreachable)
		}
		startedAt := time.Now()
		runtimeURL, release, err := router.Runtime.Acquire(ctx, model)
		if err != nil {
			err = fmt.Errorf("%w: %v", chatcompletions.ErrUnreachable, err)
			if recordRun != nil {
				recordRun(context.WithoutCancel(ctx), chatcompletions.RunRecord{
					Kind: request.GetRunKind(), Task: request.Task, BaseURL: HubRuntimeAddress, Provider: provider.Name, Model: model,
					StartedAt: startedAt, Duration: time.Since(startedAt), Outcome: chatcompletions.RunFailed, Error: err.Error(),
					StartedByOwner: chatcompletions.IsStartedByOwner(ctx),
				})
			}
			return chatcompletions.Answer{}, err
		}
		defer release()
		baseURL = runtimeURL
	}
	client := chatcompletions.NewClient(baseURL)
	client.APIKey = provider.APIKey
	client.Provider = provider.Name
	client.RecordRun = recordRun
	if provider.Kind == store.ModelProviderKindOpenRouter {
		openrouter.Configure(client)
	}
	request.Model = model
	request.SchemaNotEnforced = !provider.EnforcesSchema
	answer, err := client.CompleteJSON(ctx, request)
	if errors.Is(err, chatcompletions.ErrInvalidAnswer) && request.SchemaNotEnforced {
		return client.CompleteJSON(ctx, request)
	}
	return answer, err
}

// completeOnClaude asks Claude through the CLI, on the owner's plan. The CLI
// enforces the schema itself.
func (router *Router) completeOnClaude(ctx context.Context, provider store.ModelProvider, model string, request chatcompletions.JSONRequest,
	recordRun func(context.Context, chatcompletions.RunRecord)) (chatcompletions.Answer, error) {
	if router.Claude == nil {
		return chatcompletions.Answer{}, fmt.Errorf("%w: the hub found no Claude CLI", chatcompletions.ErrUnreachable)
	}
	client := *router.Claude
	client.Model = model
	client.Provider = provider.Name
	client.RecordRun = recordRun
	return client.CompleteJSON(ctx, request)
}
