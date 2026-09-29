// Package jobfacts reads each open job's key facts from its text with a model,
// as the active job_facts prompt describes them, and keeps them current.
package jobfacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumJobsPerPass bounds one pass; the next pass picks up the rest.
	maximumJobsPerPass = 500
	// maximumDescriptionLength keeps a posting within the model's context.
	maximumDescriptionLength = 40_000
	maximumAnswerTokens      = 2048
)

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (json.RawMessage, error)
}

type Extractor struct {
	hub       *store.Store
	client    modelClient
	modelName string
}

func NewExtractor(hub *store.Store, client modelClient, modelName string) *Extractor {
	return &Extractor{hub: hub, client: client, modelName: modelName}
}

// PassSummary counts one pass over the jobs awaiting facts.
type PassSummary struct {
	Read   int
	Failed int
}

// Run reads facts once at start and then every interval, until ctx ends.
func (extractor *Extractor) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		summary, err := extractor.ExtractOnce(ctx)
		if err != nil {
			slog.Error("job facts pass stopped", "error", err, "read", summary.Read, "failed", summary.Failed)
		} else if summary.Read > 0 || summary.Failed > 0 {
			slog.Info("job facts pass done", "read", summary.Read, "failed", summary.Failed)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ExtractOnce reads the facts of every job awaiting them, one at a time. A
// job the model fails on is logged and counted, and the others go on; an
// unreachable model server stops the pass.
func (extractor *Extractor) ExtractOnce(ctx context.Context) (PassSummary, error) {
	prompt, err := extractor.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	if err != nil {
		return PassSummary{}, fmt.Errorf("read the job_facts prompt: %w", err)
	}
	jobs, err := extractor.hub.ListJobsAwaitingFacts(ctx, prompt.ID, maximumJobsPerPass)
	if err != nil {
		return PassSummary{}, err
	}

	var summary PassSummary
	for _, job := range jobs {
		facts, err := extractor.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
			Model: extractor.modelName, System: prompt.Body, User: formatJobText(job.Job),
			SchemaName: store.AgentPromptKindJobFacts, Schema: prompt.ResultSchema, MaxTokens: maximumAnswerTokens,
			Task: chatcompletions.TaskLabel{SubjectID: &job.ID, PromptID: &prompt.ID, PromptVersion: prompt.Version},
		})
		if errors.Is(err, chatcompletions.ErrUnreachable) || ctx.Err() != nil {
			return summary, err
		}
		if err == nil {
			err = extractor.hub.SaveJobFacts(ctx, store.NewJobFacts{
				JobID: job.ID, PromptID: prompt.ID, Model: extractor.modelName, TextHash: job.TextHash, Facts: facts,
			})
		}
		if err != nil {
			summary.Failed++
			slog.Warn("job facts failed", "job", job.ID, "title", job.Title, "error", err)
			continue
		}
		summary.Read++
	}
	return summary, nil
}

// formatJobText is what the model reads: the posting's headline facts, then
// its description.
func formatJobText(job store.Job) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Title: %s\n", job.Title)
	if job.Location != "" {
		fmt.Fprintf(&text, "Location: %s\n", job.Location)
	}
	if len(job.OtherLocations) > 0 {
		fmt.Fprintf(&text, "Other locations: %s\n", strings.Join(job.OtherLocations, "; "))
	}
	if job.WorkplaceType != "" {
		fmt.Fprintf(&text, "Workplace: %s\n", job.WorkplaceType)
	}
	if job.EmploymentType != "" {
		fmt.Fprintf(&text, "Employment type: %s\n", job.EmploymentType)
	}
	description := job.Description
	if len(description) > maximumDescriptionLength {
		description = strings.ToValidUTF8(description[:maximumDescriptionLength], "")
	}
	fmt.Fprintf(&text, "\nDescription:\n%s", description)
	return text.String()
}
