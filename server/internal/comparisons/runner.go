package comparisons

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumDescriptionLength and maximumAnswerTokens match what the hub's
	// own facts reading sends, so a comparison measures the same request.
	maximumDescriptionLength = 40_000
	maximumAnswerTokens      = 2048
)

// ErrClaudeOff is a Claude stack asked of a hub that found no Claude CLI.
var ErrClaudeOff = errors.New("claude stacks are off: the hub found no Claude CLI")

type routeClient interface {
	CompleteJSONOn(ctx context.Context, providerID uuid.UUID, model string, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

// ModelClient answers a JSON request with one model.
type ModelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

// Runner answers a comparison's jobs through its stacks: route stacks on
// their provider and model, Claude stacks through the Claude CLI.
type Runner struct {
	hub    *store.Store
	router routeClient
	// NewClaudeClient returns a Claude client for a model; nil leaves Claude
	// stacks off.
	NewClaudeClient func(model string) ModelClient
}

func NewRunner(hub *store.Store, router routeClient) *Runner {
	return &Runner{hub: hub, router: router}
}

// Run answers every job the comparison's stacks haven't answered yet, with
// the latest job_facts prompt, then marks the comparison done. An answer that
// fails is kept as its error and the others go on.
func (runner *Runner) Run(ctx context.Context, comparisonID uuid.UUID) error {
	record, err := runner.hub.GetComparison(ctx, comparisonID)
	if err != nil {
		return err
	}
	if record.TaskKind != store.AgentPromptKindJobFacts {
		return fmt.Errorf("comparisons run %s only, not %s", store.AgentPromptKindJobFacts, record.TaskKind)
	}
	prompt, err := runner.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	if err != nil {
		return fmt.Errorf("read the job_facts prompt: %w", err)
	}
	answered := map[answerKey]bool{}
	for _, answer := range record.Answers {
		answered[answerKey{answer.StackID, answer.JobID}] = true
	}
	ctx = modelqueue.WithPriority(ctx, modelqueue.PriorityDispatched)
	requests := map[uuid.UUID]chatcompletions.JSONRequest{}
	for _, jobID := range record.JobIDs {
		job, err := runner.hub.GetJobForFacts(ctx, jobID)
		if err != nil {
			return err
		}
		requests[jobID] = chatcompletions.JSONRequest{
			System: prompt.Body, User: jobfacts.FormatJobText(job.Job, maximumDescriptionLength),
			SchemaName: store.AgentPromptKindJobFacts, Schema: prompt.ResultSchema, Examples: prompt.Examples, MaxTokens: maximumAnswerTokens,
			Task: chatcompletions.TaskLabel{SubjectID: &job.ID, PromptID: &prompt.ID, PromptVersion: prompt.Version},
		}
	}
	for _, stack := range record.Stacks {
		if stack.Source == store.ComparisonSourceImported {
			continue
		}
		var unanswered []uuid.UUID
		for _, jobID := range record.JobIDs {
			if !answered[answerKey{stack.ID, jobID}] {
				unanswered = append(unanswered, jobID)
			}
		}
		if err := runner.answerJobs(ctx, comparisonID, stack, unanswered, requests); err != nil {
			return err
		}
	}
	return runner.hub.FinishComparison(ctx, comparisonID)
}

// answerJobs answers the jobs through one stack, saving each answer as it
// lands. A route stack asks for every job at once, so the requests all wait
// in the model queue together: the queue then serves them back to back on
// the loaded model, where one at a time would let the hub's own work swap
// its model back in between. Claude stacks go one at a time.
func (runner *Runner) answerJobs(ctx context.Context, comparisonID uuid.UUID, stack store.ComparisonStack, jobIDs []uuid.UUID, requests map[uuid.UUID]chatcompletions.JSONRequest) error {
	saveErrors := make([]error, len(jobIDs))
	answerJob := func(index int) {
		jobID := jobIDs[index]
		result := store.ComparisonAnswer{StackID: stack.ID, JobID: jobID}
		answer, err := runner.answer(ctx, stack, requests[jobID])
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, drain.ErrDraining) {
			// No answer: the comparison stays running, and resumes when
			// the server starts again.
			saveErrors[index] = err
			return
		}
		if err != nil {
			result.Error = err.Error()
			slog.Warn("comparison answer failed", "comparison", comparisonID, "stack", stack.Label, "job", jobID, "error", err)
		} else {
			result.Answer = answer.Object
		}
		saveErrors[index] = runner.hub.SaveComparisonAnswer(ctx, result)
	}
	if stack.Source == store.ComparisonSourceRoute {
		var group sync.WaitGroup
		for index := range jobIDs {
			group.Go(func() { answerJob(index) })
		}
		group.Wait()
	} else {
		for index := range jobIDs {
			answerJob(index)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.Join(saveErrors...)
}

// RunUnfinished resumes the comparisons a stopped server left running.
func (runner *Runner) RunUnfinished(ctx context.Context) {
	ids, err := runner.hub.ListRunningComparisonIDs(ctx)
	if err != nil {
		slog.Error("list running comparisons", "error", err)
		return
	}
	for _, id := range ids {
		if err := runner.Run(ctx, id); err != nil {
			slog.Warn("comparison stopped", "comparison", id, "error", err)
		}
	}
}

func (runner *Runner) answer(ctx context.Context, stack store.ComparisonStack, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	switch stack.Source {
	case store.ComparisonSourceRoute:
		if stack.ProviderID == nil {
			return chatcompletions.Answer{}, errors.New("the stack's provider was removed")
		}
		return runner.router.CompleteJSONOn(ctx, *stack.ProviderID, stack.Model, request)
	case store.ComparisonSourceClaude:
		if runner.NewClaudeClient == nil {
			return chatcompletions.Answer{}, ErrClaudeOff
		}
		return runner.NewClaudeClient(stack.Model).CompleteJSON(ctx, request)
	default:
		return chatcompletions.Answer{}, fmt.Errorf("a %s stack can't be run", stack.Source)
	}
}
