// Package jobbriefs writes each good or unclear job's pre-brief with a
// model, from the owner's knowledge base, and keeps it current.
package jobbriefs

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumBriefsPerPass bounds one pass, so a new prompt version or a
	// knowledge base edit reaches the next pass within minutes.
	maximumBriefsPerPass = 10
	// maximumDescriptionLength keeps the posting beside the knowledge base in
	// a local model's context.
	maximumDescriptionLength = 12_000
	maximumAnswerTokens      = 1500
)

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

type Writer struct {
	hub    *store.Store
	client modelClient
	rates  jobfit.RateSource
}

func NewWriter(hub *store.Store, client modelClient, rates jobfit.RateSource) *Writer {
	return &Writer{hub: hub, client: client, rates: rates}
}

// PassSummary counts one pass over the jobs to brief.
type PassSummary struct {
	Written int
	Failed  int
	// Skipped are poor fits, which get no brief.
	Skipped int
}

// Run writes pre-briefs once at start and then every interval, until ctx ends.
func (writer *Writer) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		summary, err := writer.WriteOnce(ctx)
		if err != nil {
			slog.Error("job briefs pass stopped", "error", err, "written", summary.Written, "failed", summary.Failed)
		} else if summary.Written > 0 || summary.Failed > 0 {
			slog.Info("job briefs pass done", "written", summary.Written, "failed", summary.Failed, "skipped", summary.Skipped)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// WriteOnce writes the pre-briefs of up to maximumBriefsPerPass good or
// unclear jobs that have none, or an outdated one. Without a job_brief prompt it does nothing. A job the
// model fails on is logged and counted; an unreachable model server stops
// the pass.
func (writer *Writer) WriteOnce(ctx context.Context) (PassSummary, error) {
	prompt, err := writer.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobBrief)
	if errors.Is(err, store.ErrAgentPromptNotFound) {
		return PassSummary{}, nil
	}
	if err != nil {
		return PassSummary{}, fmt.Errorf("read the job_brief prompt: %w", err)
	}
	knowledgeHash, err := writer.hub.GetKnowledgeHash(ctx)
	if err != nil {
		return PassSummary{}, err
	}
	jobs, err := writer.hub.ListJobsToBrief(ctx, prompt.ID, knowledgeHash)
	if err != nil || len(jobs) == 0 {
		return PassSummary{}, err
	}
	rendered, err := prompts.RenderJobBriefPrompt(ctx, writer.hub, prompt)
	if err != nil {
		return PassSummary{}, err
	}
	criteria, rates, err := jobfit.ReadInputs(ctx, writer.hub, writer.rates)
	if err != nil {
		return PassSummary{}, err
	}

	var summary PassSummary
	var fitting []judgedJob
	for _, job := range jobs {
		level := jobfit.Judge(job.Job, job.Facts, criteria, rates).Level
		if level == jobfit.LevelPoor {
			summary.Skipped++
			continue
		}
		fitting = append(fitting, judgedJob{JobToBrief: job, level: level})
	}
	// Jobs without a brief come first, as listed; within each group, good fits
	// go before unclear ones, since they're the likeliest to be pursued.
	slices.SortStableFunc(fitting, func(a, b judgedJob) int {
		if a.HasOutdatedBrief != b.HasOutdatedBrief {
			return cmp.Compare(convertBoolToInt(a.HasOutdatedBrief), convertBoolToInt(b.HasOutdatedBrief))
		}
		return cmp.Compare(convertBoolToInt(a.level != jobfit.LevelGood), convertBoolToInt(b.level != jobfit.LevelGood))
	})
	for _, judged := range fitting[:min(len(fitting), maximumBriefsPerPass)] {
		job := judged.JobToBrief
		err := writer.writePreBrief(ctx, job, prompt, rendered, knowledgeHash)
		if errors.Is(err, chatcompletions.ErrUnreachable) || ctx.Err() != nil {
			return summary, err
		}
		if err != nil {
			summary.Failed++
			slog.Warn("job brief failed", "job", job.Job.ID, "title", job.Job.Title, "error", err)
			continue
		}
		summary.Written++
	}
	return summary, nil
}

// judgedJob is a job to brief with its fit level, which orders the pass.
type judgedJob struct {
	store.JobToBrief
	level jobfit.Level
}

func convertBoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// briefAnswer is the brief as the job_brief schema asks the model for it,
// citing entries by their references.
type briefAnswer struct {
	Match      string        `json:"match"`
	Reason     string        `json:"reason"`
	Strengths  []pointAnswer `json:"strengths"`
	Weaknesses []pointAnswer `json:"weaknesses"`
}

type pointAnswer struct {
	Point   string   `json:"point"`
	Entries []string `json:"entries"`
}

func (writer *Writer) writePreBrief(ctx context.Context, job store.JobToBrief, prompt store.AgentPrompt, rendered prompts.JobBriefPrompt, knowledgeHash string) error {
	answer, err := writer.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		System: rendered.Body, User: formatBriefInput(job),
		SchemaName: store.AgentPromptKindJobBrief, Schema: prompt.ResultSchema, Examples: prompt.Examples, MaxTokens: maximumAnswerTokens,
		Task: chatcompletions.TaskLabel{SubjectID: &job.Job.ID, PromptID: &prompt.ID, PromptVersion: prompt.Version},
	})
	if err != nil {
		return err
	}
	var brief briefAnswer
	if err := json.Unmarshal(answer.Object, &brief); err != nil {
		return fmt.Errorf("read the brief: %w", err)
	}
	return writer.hub.SaveJobBrief(ctx, store.JobBrief{
		JobID: job.Job.ID, Tier: store.JobBriefTierPre, PromptID: prompt.ID, Model: answer.Model,
		Match: brief.Match, Reason: strings.TrimSpace(brief.Reason),
		Strengths: convertPointsToEntries(brief.Strengths, rendered.EntryRefs), Weaknesses: convertPointsToEntries(brief.Weaknesses, rendered.EntryRefs),
		KnowledgeHash: knowledgeHash,
	})
}

// convertPointsToEntries turns each point's references into the entries they
// name, leaving out references the knowledge base doesn't have.
func convertPointsToEntries(points []pointAnswer, refs map[string]uuid.UUID) []store.JobBriefPoint {
	converted := make([]store.JobBriefPoint, 0, len(points))
	for _, point := range points {
		entryIDs := []uuid.UUID{}
		for _, ref := range point.Entries {
			if id, known := refs[strings.Trim(strings.TrimSpace(ref), "[]")]; known {
				entryIDs = append(entryIDs, id)
			}
		}
		converted = append(converted, store.JobBriefPoint{Point: strings.TrimSpace(point.Point), EntryIDs: entryIDs})
	}
	return converted
}

// formatBriefInput is the job as the brief reads it: its company, the facts
// already read from it, and the posting.
func formatBriefInput(job store.JobToBrief) string {
	var text strings.Builder
	if job.CompanyName != nil {
		fmt.Fprintf(&text, "Company: %s\n", *job.CompanyName)
	}
	if flattened := store.FlattenJobFactsToJSON(job.Facts); len(flattened) > 0 {
		fmt.Fprintf(&text, "Facts already read from the posting: %s\n", flattened)
	}
	text.WriteString(jobfacts.FormatJobText(job.Job, maximumDescriptionLength))
	return text.String()
}
