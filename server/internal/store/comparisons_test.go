package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestAComparisonKeepsItsAnswersAndVerdicts(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	first, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Product Engineer", URL: "https://acme.com/jobs/1", Description: "Build things."})
	second, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Staff Engineer", URL: "https://acme.com/jobs/2", Description: "Lead things."})
	hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "No text", URL: "https://acme.com/jobs/3"})
	provider, err := hub.SaveModelProvider(ctx, owner, nil, store.ModelProviderInput{Name: "Local", BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := hub.CreateComparison(ctx, owner, store.NewComparison{
		TaskKind: store.AgentPromptKindJobFacts, Title: "One stack",
		Stacks: []store.ComparisonStack{{Label: "Key", Source: store.ComparisonSourceImported}}, JobIDs: []uuid.UUID{first.ID},
	}); err == nil {
		t.Fatal("a comparison with one stack was created")
	}
	if _, err := hub.CreateComparison(ctx, owner, store.NewComparison{
		TaskKind: store.AgentPromptKindJobFacts, Title: "Missing job",
		Stacks: []store.ComparisonStack{{Label: "A", Source: store.ComparisonSourceImported}, {Label: "B", Source: store.ComparisonSourceImported}},
		JobIDs: []uuid.UUID{uuid.New()},
	}); !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("with a missing job: %v, want ErrJobNotFound", err)
	}

	comparison, err := hub.CreateComparison(ctx, owner, store.NewComparison{
		TaskKind: store.AgentPromptKindJobFacts, Title: "Key against local",
		Stacks: []store.ComparisonStack{
			{Label: "Key", Source: store.ComparisonSourceImported},
			{Label: "Local 9B", Source: store.ComparisonSourceRoute, ProviderID: &provider.ID, Model: "qwen-9b"},
		},
		JobIDs: []uuid.UUID{second.ID, first.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	key, local := comparison.Stacks[0], comparison.Stacks[1]
	if running, _ := hub.ListRunningComparisonIDs(ctx); len(running) != 1 || running[0] != comparison.ID {
		t.Fatalf("running = %v", running)
	}
	answers := []store.ComparisonAnswer{
		{StackID: key.ID, JobID: first.ID, Answer: json.RawMessage(`{"stack":"Go"}`)},
		{StackID: local.ID, JobID: first.ID, Error: "the model server didn't answer"},
		{StackID: local.ID, JobID: first.ID, Answer: json.RawMessage(`{"stack":"Rust"}`)},
	}
	for _, answer := range answers {
		if err := hub.SaveComparisonAnswer(ctx, answer); err != nil {
			t.Fatal(err)
		}
	}
	if err := hub.SaveComparisonVerdict(ctx, comparison.ID, store.ComparisonVerdict{StackID: local.ID, JobID: first.ID, Field: "stack", Verdict: "wrong"}); err != nil {
		t.Fatal(err)
	}
	if err := hub.SaveComparisonVerdict(ctx, comparison.ID, store.ComparisonVerdict{StackID: local.ID, JobID: first.ID, Field: "stack", Verdict: "right"}); err != nil {
		t.Fatal(err)
	}
	if err := hub.SaveComparisonVerdict(ctx, uuid.New(), store.ComparisonVerdict{StackID: local.ID, JobID: first.ID, Field: "stack", Verdict: "right"}); !errors.Is(err, store.ErrComparisonNotFound) {
		t.Errorf("a verdict through another comparison: %v", err)
	}
	if err := hub.FinishComparison(ctx, comparison.ID); err != nil {
		t.Fatal(err)
	}

	record, err := hub.GetComparison(ctx, comparison.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != store.ComparisonStatusDone || len(record.Stacks) != 2 || record.Stacks[1].Model != "qwen-9b" || *record.Stacks[1].ProviderID != provider.ID {
		t.Fatalf("record = %+v", record.Comparison)
	}
	if len(record.JobIDs) != 2 || record.JobIDs[0] != second.ID {
		t.Errorf("jobs = %v, want the order they were given", record.JobIDs)
	}
	if len(record.Answers) != 2 {
		t.Fatalf("answers = %+v, want one per stack and job", record.Answers)
	}
	for _, answer := range record.Answers {
		if answer.StackID == local.ID && (answer.Error != "" || string(answer.Answer) != `{"stack": "Rust"}`) {
			t.Errorf("the local answer = %+v, want the second save", answer)
		}
	}
	if len(record.Verdicts) != 1 || record.Verdicts[0].Verdict != "right" {
		t.Errorf("verdicts = %+v", record.Verdicts)
	}
	if list, _ := hub.ListComparisons(ctx); len(list) != 1 || list[0].Title != "Key against local" {
		t.Errorf("list = %+v", list)
	}
	if running, _ := hub.ListRunningComparisonIDs(ctx); len(running) != 0 {
		t.Errorf("still running: %v", running)
	}
	if fresh, _ := hub.ListNewestJobIDsWithText(ctx, 5); len(fresh) != 2 {
		t.Errorf("jobs with text = %v, want the two with a description", fresh)
	}
}
