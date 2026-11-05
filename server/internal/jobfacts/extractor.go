// Package jobfacts reads each open job's key facts from its text with a model,
// as the active job_facts prompt describes them, and keeps them current.
package jobfacts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
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
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

type Extractor struct {
	hub    *store.Store
	client modelClient
	// Now is the clock the backoff reads; time.Now when nil.
	Now     func() time.Time
	backoff backoff
}

func NewExtractor(hub *store.Store, client modelClient) *Extractor {
	return &Extractor{hub: hub, client: client}
}

// PassSummary counts one pass over the jobs awaiting facts. Waiting counts
// the jobs it left for later, as they failed recently. PausedUntil is set
// when failures in a row paused the background, which reads no job until
// then; LastError is the last failure's.
type PassSummary struct {
	Read        int
	Failed      int
	Waiting     int
	PausedUntil time.Time
	LastError   error
}

// Run reads facts once at start and then every interval, until ctx ends.
func (extractor *Extractor) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			summary, err := extractor.ExtractOnce(ctx)
			switch {
			case err != nil:
				slog.Error("job facts pass stopped", "error", err, "read", summary.Read, "failed", summary.Failed)
			case !summary.PausedUntil.IsZero() && summary.Failed > 0:
				slog.Warn("job facts paused after failures in a row", "until", summary.PausedUntil.Format(time.RFC3339),
					"read", summary.Read, "failed", summary.Failed, "error", summary.LastError)
			case summary.Read > 0 || summary.Failed > 0:
				slog.Info("job facts pass done", "read", summary.Read, "failed", summary.Failed)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ExtractOnce reads the facts of every job awaiting them, one at a time. A
// job the model fails on is logged and counted, and the others go on; the
// job waits before its next try, 15 minutes at first and doubling. Three
// failures in a row stop the pass and pause it, 5 minutes at first and
// doubling up to an hour, so a model failing every call isn't retried in a
// loop. An unreachable model server stops the pass.
func (extractor *Extractor) ExtractOnce(ctx context.Context) (PassSummary, error) {
	if pausedUntil := extractor.backoff.pausedUntilAt(extractor.now()); !pausedUntil.IsZero() {
		return PassSummary{PausedUntil: pausedUntil}, nil
	}
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
		if extractor.backoff.isWaiting(job.ID, extractor.now()) {
			summary.Waiting++
			continue
		}
		err := extractor.readJobFacts(ctx, job, prompt)
		if errors.Is(err, chatcompletions.ErrUnreachable) || ctx.Err() != nil {
			return summary, err
		}
		if err != nil {
			summary.Failed++
			summary.LastError = err
			slog.Warn("job facts failed", "job", job.ID, "title", job.Title, "error", err)
			if summary.PausedUntil = extractor.backoff.failed(job.ID, extractor.now()); !summary.PausedUntil.IsZero() {
				return summary, nil
			}
			continue
		}
		extractor.backoff.succeeded(job.ID)
		summary.Read++
	}
	return summary, nil
}

func (extractor *Extractor) now() time.Time {
	if extractor.Now == nil {
		return time.Now()
	}
	return extractor.Now()
}

// ReadJobNow reads one job's facts with the latest prompt as a run the owner
// dispatched: it goes ahead of background work, even while that is paused,
// or waits after failures. Its success ends that wait.
func (extractor *Extractor) ReadJobNow(ctx context.Context, jobID uuid.UUID) error {
	prompt, err := extractor.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	if err != nil {
		return fmt.Errorf("read the job_facts prompt: %w", err)
	}
	job, err := extractor.hub.GetJobForFacts(ctx, jobID)
	if err != nil {
		return err
	}
	ctx = chatcompletions.WithStartedByOwner(modelqueue.WithPriority(ctx, modelqueue.PriorityDispatched))
	if err := extractor.readJobFacts(ctx, job, prompt); err != nil {
		return err
	}
	extractor.backoff.succeeded(job.ID)
	return nil
}

// readJobFacts asks the model for the job's facts and saves them.
func (extractor *Extractor) readJobFacts(ctx context.Context, job store.JobAwaitingFacts, prompt store.AgentPrompt) error {
	answer, err := extractor.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		System: prompt.Body, User: FormatJobText(job.Job, maximumDescriptionLength),
		SchemaName: store.AgentPromptKindJobFacts, Schema: prompt.ResultSchema, Examples: prompt.Examples, MaxTokens: maximumAnswerTokens,
		Task: chatcompletions.TaskLabel{SubjectID: &job.ID, PromptID: &prompt.ID, PromptVersion: prompt.Version},
	})
	if err != nil {
		return err
	}
	return extractor.hub.SaveJobFacts(ctx, store.NewJobFacts{
		JobID: job.ID, PromptID: prompt.ID, Model: answer.Model, TextHash: job.TextHash, Facts: answer.Object,
	})
}

// FormatJobText is what a model reads of a posting: its headline facts, then
// its description, cut at maximumDescriptionLength bytes.
func FormatJobText(job store.Job, maximumDescriptionLength int) string {
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
