// Package modelrouter sends each model request to the provider and model its
// kind of task is routed to, with the route's fallback when that provider is
// unreachable.
package modelrouter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
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
	// context, else its kind's.
	Queue queue
	// Runtime serves routes to a hub runtime provider; nil leaves them
	// unreachable.
	Runtime runtime
	// RecordRun, when set, hears about every attempt, fallbacks and retries
	// included.
	RecordRun func(context.Context, chatcompletions.RunRecord)
}

func New(hub *store.Store) *Router {
	return &Router{hub: hub}
}

// CompleteJSON answers request on the model its kind (request.SchemaName) is
// routed to. A provider that can't enforce a schema gets one retry for an
// invalid answer.
func (router *Router) CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	route, err := router.hub.GetTaskRoute(ctx, request.SchemaName)
	if errors.Is(err, store.ErrTaskRouteNotFound) {
		return chatcompletions.Answer{}, fmt.Errorf("no model is routed for %s", request.SchemaName)
	}
	if err != nil {
		return chatcompletions.Answer{}, err
	}
	if router.Queue != nil {
		release, err := router.Queue.Wait(ctx, modelqueue.Ticket{
			Kind: request.SchemaName, SubjectID: request.Task.SubjectID, Model: route.Model,
			Priority: modelqueue.GetPriority(ctx, getKindPriority(request.SchemaName)),
		})
		if err != nil {
			return chatcompletions.Answer{}, err
		}
		defer release()
	}
	answer, err := router.completeOnProvider(ctx, route.ProviderID, route.Model, request)
	if errors.Is(err, chatcompletions.ErrUnreachable) && route.FallbackProviderID != nil {
		return router.completeOnProvider(ctx, *route.FallbackProviderID, route.FallbackModel, request)
	}
	return answer, err
}

// getKindPriority is how soon a kind of task runs when nobody asked for it:
// sorting shows up in the owner's inbox views, facts can wait.
func getKindPriority(kind string) modelqueue.Priority {
	if kind == store.AgentPromptKindMailTriage || kind == store.AgentPromptKindLinkedInConversation {
		return modelqueue.PrioritySorting
	}
	return modelqueue.PriorityBackground
}

func (router *Router) completeOnProvider(ctx context.Context, providerID uuid.UUID, model string, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	provider, err := router.hub.GetModelProvider(ctx, providerID)
	if err != nil {
		return chatcompletions.Answer{}, fmt.Errorf("read the model provider: %w", err)
	}
	baseURL := provider.BaseURL
	if provider.Kind == store.ModelProviderKindHubRuntime {
		if router.Runtime == nil {
			return chatcompletions.Answer{}, fmt.Errorf("%w: the hub's model runtime isn't set up", chatcompletions.ErrUnreachable)
		}
		startedAt := time.Now()
		runtimeURL, release, err := router.Runtime.Acquire(ctx, model)
		if err != nil {
			err = fmt.Errorf("%w: %v", chatcompletions.ErrUnreachable, err)
			if router.RecordRun != nil {
				router.RecordRun(context.WithoutCancel(ctx), chatcompletions.RunRecord{
					Kind: request.SchemaName, Task: request.Task, BaseURL: HubRuntimeAddress, Model: model,
					StartedAt: startedAt, Duration: time.Since(startedAt), Outcome: chatcompletions.RunFailed, Error: err.Error(),
				})
			}
			return chatcompletions.Answer{}, err
		}
		defer release()
		baseURL = runtimeURL
	}
	client := chatcompletions.NewClient(baseURL)
	client.APIKey = provider.APIKey
	client.RecordRun = router.RecordRun
	request.Model = model
	request.SchemaNotEnforced = !provider.EnforcesSchema
	answer, err := client.CompleteJSON(ctx, request)
	if errors.Is(err, chatcompletions.ErrInvalidAnswer) && request.SchemaNotEnforced {
		return client.CompleteJSON(ctx, request)
	}
	return answer, err
}
