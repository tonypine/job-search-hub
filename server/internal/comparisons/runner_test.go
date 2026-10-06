package comparisons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// fakeModel answers every request with answer, or fails with err, and counts
// what it was asked.
type fakeModel struct {
	answer string
	err    error
	models []string
}

func (model *fakeModel) CompleteJSONOn(_ context.Context, _ uuid.UUID, name string, _ chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	model.models = append(model.models, name)
	return chatcompletions.Answer{Object: json.RawMessage(model.answer), Model: name}, model.err
}

func (model *fakeModel) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	model.models = append(model.models, "claude")
	return chatcompletions.Answer{Object: json.RawMessage(model.answer)}, model.err
}

func TestARunAnswersEachStackOnceAndKeepsFailures(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Product Engineer", URL: "https://acme.com/jobs/1", Description: "Build things."})
	provider, _ := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "Local", BaseURL: "http://localhost:1"})
	comparison, err := hub.CreateComparison(ctx, owner, store.NewComparison{
		TaskKind: store.AgentPromptKindJobFacts, Title: "Local against Sonnet", JobIDs: []uuid.UUID{job.ID},
		Stacks: []store.ComparisonStack{
			{Label: "Key", Source: store.ComparisonSourceImported},
			{Label: "Local", Source: store.ComparisonSourceRoute, ProviderID: &provider.ID, Model: "qwen-9b"},
			{Label: "Sonnet", Source: store.ComparisonSourceClaude, Model: "sonnet"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	local := &fakeModel{answer: `{"stack":"Go"}`}
	claude := &fakeModel{err: errors.New("claude exited 1")}
	var claudeModels []string
	runner := NewRunner(hub, local)
	runner.NewClaudeClient = func(model string) ModelClient {
		claudeModels = append(claudeModels, model)
		return claude
	}

	if err := runner.Run(ctx, comparison.ID); err != nil {
		t.Fatal(err)
	}
	record, _ := hub.GetComparison(ctx, comparison.ID)
	if record.Status != store.ComparisonStatusDone || len(record.Answers) != 2 {
		t.Fatalf("record = %+v, answers = %+v", record.Comparison, record.Answers)
	}
	for _, answer := range record.Answers {
		switch answer.StackID {
		case comparison.Stacks[1].ID:
			if string(answer.Answer) != `{"stack": "Go"}` {
				t.Errorf("local answer = %s", answer.Answer)
			}
		case comparison.Stacks[2].ID:
			if answer.Error != "claude exited 1" {
				t.Errorf("claude answer = %+v, want its error", answer)
			}
		}
	}
	if len(local.models) != 1 || local.models[0] != "qwen-9b" || len(claudeModels) != 1 || claudeModels[0] != "sonnet" {
		t.Errorf("asked local %v and claude %v", local.models, claudeModels)
	}

	if err := runner.Run(ctx, comparison.ID); err != nil {
		t.Fatal(err)
	}
	if len(local.models) != 1 || len(claude.models) != 1 {
		t.Errorf("a second run asked again: local %v, claude %v", local.models, claude.models)
	}
}

func TestAClaudeRunRefusedByADrainLeavesTheComparisonToResume(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Product Engineer", URL: "https://acme.com/jobs/1", Description: "Build things."})
	comparison, err := hub.CreateComparison(ctx, owner, store.NewComparison{
		TaskKind: store.AgentPromptKindJobFacts, Title: "Sonnet", JobIDs: []uuid.UUID{job.ID},
		Stacks: []store.ComparisonStack{{Label: "Sonnet", Source: store.ComparisonSourceClaude, Model: "sonnet"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	claude := &fakeModel{err: fmt.Errorf("%w: %w", chatcompletions.ErrUnreachable, drain.ErrDraining)}
	runner := NewRunner(hub, &fakeModel{})
	runner.NewClaudeClient = func(string) ModelClient { return claude }

	if err := runner.Run(ctx, comparison.ID); !errors.Is(err, drain.ErrDraining) {
		t.Fatalf("run while draining = %v, want ErrDraining", err)
	}
	record, _ := hub.GetComparison(ctx, comparison.ID)
	if record.Status == store.ComparisonStatusDone || len(record.Answers) != 0 {
		t.Fatalf("record = %+v, answers = %+v, want it running without answers", record.Comparison, record.Answers)
	}

	claude.err, claude.answer = nil, `{"stack":"Go"}`
	if err := runner.Run(ctx, comparison.ID); err != nil {
		t.Fatal(err)
	}
	record, _ = hub.GetComparison(ctx, comparison.ID)
	if record.Status != store.ComparisonStatusDone || len(record.Answers) != 1 || record.Answers[0].Error != "" {
		t.Fatalf("record after the restart = %+v, answers = %+v", record.Comparison, record.Answers)
	}
}

// gatheringModel answers only once expected requests are waiting at the
// same time, and fails them all when they don't arrive together.
type gatheringModel struct {
	expected int
	mutex    sync.Mutex
	arrived  int
	gathered chan struct{}
}

func (model *gatheringModel) CompleteJSONOn(_ context.Context, _ uuid.UUID, name string, _ chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	model.mutex.Lock()
	model.arrived++
	if model.arrived == model.expected {
		close(model.gathered)
	}
	model.mutex.Unlock()
	select {
	case <-model.gathered:
		return chatcompletions.Answer{Object: json.RawMessage(`{"stack":"Go"}`), Model: name}, nil
	case <-time.After(2 * time.Second):
		return chatcompletions.Answer{}, errors.New("asked one at a time")
	}
}

func TestARouteStacksJobsWaitInTheQueueTogether(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	var jobIDs []uuid.UUID
	for _, url := range []string{"https://acme.com/jobs/1", "https://acme.com/jobs/2", "https://acme.com/jobs/3"} {
		job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: url, Description: "Build things."})
		jobIDs = append(jobIDs, job.ID)
	}
	provider, _ := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "Local", BaseURL: "http://localhost:1"})
	comparison, err := hub.CreateComparison(ctx, owner, store.NewComparison{
		TaskKind: store.AgentPromptKindJobFacts, Title: "Local", JobIDs: jobIDs,
		Stacks: []store.ComparisonStack{
			{Label: "Key", Source: store.ComparisonSourceImported},
			{Label: "Local", Source: store.ComparisonSourceRoute, ProviderID: &provider.ID, Model: "qwen-9b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := NewRunner(hub, &gatheringModel{expected: 3, gathered: make(chan struct{})}).Run(ctx, comparison.ID); err != nil {
		t.Fatal(err)
	}
	record, _ := hub.GetComparison(ctx, comparison.ID)
	for _, answer := range record.Answers {
		if answer.Error != "" {
			t.Errorf("job %s: %s", answer.JobID, answer.Error)
		}
	}
	if len(record.Answers) != 3 {
		t.Errorf("answers = %d, want 3", len(record.Answers))
	}
}
