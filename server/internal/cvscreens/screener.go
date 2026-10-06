// Package cvscreens reads each pursued job's tailored CV as the job's
// recruiter would, with a routed model, and keeps the likely reasons to
// reject it so they can be answered before applying.
package cvscreens

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumScreensPerPass bounds one pass; the next pass takes the rest.
	maximumScreensPerPass = 5
	// maximumDescriptionLength keeps the posting and the CV within the
	// model's context.
	maximumDescriptionLength = 30_000
	maximumAnswerTokens      = 2048
)

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

type Screener struct {
	hub    *store.Store
	client modelClient
}

func NewScreener(hub *store.Store, client modelClient) *Screener {
	return &Screener{hub: hub, client: client}
}

// PassSummary counts one pass over the CVs awaiting a screen.
type PassSummary struct {
	Screened int
	Failed   int
}

// Run screens the CVs awaiting it at start and then every interval.
func (screener *Screener) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			summary, err := screener.ScreenOnce(ctx)
			if err != nil {
				slog.Error("cv screens pass stopped", "error", err, "screened", summary.Screened, "failed", summary.Failed)
			} else if summary.Screened > 0 || summary.Failed > 0 {
				slog.Info("cv screens pass done", "screened", summary.Screened, "failed", summary.Failed)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ScreenOnce screens up to maximumScreensPerPass CVs. A CV the model fails on
// is logged and counted; an unreachable model server stops the pass. With
// no recruiter_screen prompt saved yet, there is nothing to do.
func (screener *Screener) ScreenOnce(ctx context.Context) (PassSummary, error) {
	prompt, err := screener.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindRecruiterScreen)
	if errors.Is(err, store.ErrAgentPromptNotFound) {
		return PassSummary{}, nil
	}
	if err != nil {
		return PassSummary{}, err
	}
	cvs, err := screener.hub.ListCVsAwaitingScreen(ctx, prompt.ID, maximumScreensPerPass)
	if err != nil {
		return PassSummary{}, err
	}
	var summary PassSummary
	for _, cv := range cvs {
		err := screener.screenCV(ctx, cv, prompt)
		if errors.Is(err, chatcompletions.ErrUnreachable) || ctx.Err() != nil {
			return summary, err
		}
		if err != nil {
			summary.Failed++
			slog.Warn("cv screen failed", "job", cv.JobID, "error", err)
			continue
		}
		summary.Screened++
	}
	return summary, nil
}

func (screener *Screener) screenCV(ctx context.Context, cv store.CV, prompt store.AgentPrompt) error {
	job, err := screener.hub.GetJobForFacts(ctx, *cv.JobID)
	if err != nil {
		return err
	}
	input, err := formatScreenInput(job.Job, cv)
	if err != nil {
		return err
	}
	answer, err := screener.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		System: prompt.Body, User: input, SchemaName: store.AgentPromptKindRecruiterScreen, Schema: prompt.ResultSchema,
		Examples: prompt.Examples, MaxTokens: maximumAnswerTokens,
		Task: chatcompletions.TaskLabel{SubjectID: cv.JobID, PromptID: &prompt.ID, PromptVersion: prompt.Version},
	})
	if err != nil {
		return err
	}
	return screener.hub.SaveCVScreen(ctx, store.CVScreen{
		JobID: *cv.JobID, CVID: cv.ID, CVUpdatedAt: cv.UpdatedAt, PromptID: prompt.ID, Model: answer.Model, Screen: answer.Object,
	})
}

// formatScreenInput is what the recruiter reads: the posting, then the CV
// as JSON Resume, fenced so neither reads as instructions.
func formatScreenInput(job store.Job, cv store.CV) (string, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cv.Content); err != nil {
		return "", err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "## The posting\n\n%s\n\n", jobfacts.FormatJobText(job, maximumDescriptionLength))
	fmt.Fprintf(&text, "## The CV sent for it\n\n```json\n%s```\n", encoded.String())
	return text.String(), nil
}
