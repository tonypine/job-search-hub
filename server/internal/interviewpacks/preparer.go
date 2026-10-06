// Package interviewpacks prepares each pursued job's interview pack with a
// routed model: the questions the posting suggests, the confirmed cases to
// tell for each, and how to answer honestly where the role touches a market
// gap.
package interviewpacks

import (
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
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumPacksPerPass bounds one pass; the next pass takes the rest.
	maximumPacksPerPass      = 3
	maximumDescriptionLength = 30_000
	maximumAnswerTokens      = 4096
)

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

type Preparer struct {
	hub    *store.Store
	client modelClient
}

func NewPreparer(hub *store.Store, client modelClient) *Preparer {
	return &Preparer{hub: hub, client: client}
}

// PassSummary counts one pass over the jobs awaiting a pack.
type PassSummary struct {
	Prepared int
	Failed   int
}

// Run prepares the packs awaiting at start and then every interval.
func (preparer *Preparer) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			summary, err := preparer.PrepareOnce(ctx)
			if err != nil {
				slog.Error("interview packs pass stopped", "error", err, "prepared", summary.Prepared, "failed", summary.Failed)
			} else if summary.Prepared > 0 || summary.Failed > 0 {
				slog.Info("interview packs pass done", "prepared", summary.Prepared, "failed", summary.Failed)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// PrepareOnce prepares up to maximumPacksPerPass packs. With no
// interview_prep prompt saved yet, there is nothing to do.
func (preparer *Preparer) PrepareOnce(ctx context.Context) (PassSummary, error) {
	prompt, err := preparer.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindInterviewPrep)
	if errors.Is(err, store.ErrAgentPromptNotFound) {
		return PassSummary{}, nil
	}
	if err != nil {
		return PassSummary{}, err
	}
	knowledgeHash, err := preparer.hub.GetKnowledgeHash(ctx)
	if err != nil {
		return PassSummary{}, err
	}
	jobIDs, err := preparer.hub.ListJobsAwaitingInterviewPack(ctx, prompt.ID, knowledgeHash, maximumPacksPerPass)
	if err != nil || len(jobIDs) == 0 {
		return PassSummary{}, err
	}
	rendered, err := prompts.RenderInterviewPrepPrompt(ctx, preparer.hub, prompt)
	if err != nil {
		return PassSummary{}, err
	}
	entries, err := preparer.hub.ListProfileEntries(ctx, store.ProfileEntryFilter{})
	if err != nil {
		return PassSummary{}, err
	}
	gaps, err := preparer.hub.ListMarketGaps(ctx)
	if err != nil {
		return PassSummary{}, err
	}
	var summary PassSummary
	for _, jobID := range jobIDs {
		err := preparer.preparePack(ctx, jobID, prompt, rendered, knowledgeHash, entries, gaps)
		if errors.Is(err, chatcompletions.ErrUnreachable) || ctx.Err() != nil {
			return summary, err
		}
		if err != nil {
			summary.Failed++
			slog.Warn("interview pack failed", "job", jobID, "error", err)
			continue
		}
		summary.Prepared++
	}
	return summary, nil
}

func (preparer *Preparer) preparePack(ctx context.Context, jobID uuid.UUID, prompt store.AgentPrompt, rendered prompts.InterviewPrepPrompt,
	knowledgeHash string, entries []store.ProfileEntry, gaps []store.MarketGap) error {
	job, err := preparer.hub.GetJobForFacts(ctx, jobID)
	if err != nil {
		return err
	}
	answer, err := preparer.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		System: rendered.Body, User: formatPackInput(job.Job, gaps), SchemaName: store.AgentPromptKindInterviewPrep,
		Schema: prompt.ResultSchema, Examples: prompt.Examples, MaxTokens: maximumAnswerTokens,
		Task: chatcompletions.TaskLabel{SubjectID: &jobID, PromptID: &prompt.ID, PromptVersion: prompt.Version},
	})
	if err != nil {
		return err
	}
	pack, err := convertAnswerToPack(answer.Object, rendered.EntryRefs, entries)
	if err != nil {
		return err
	}
	return preparer.hub.SaveInterviewPack(ctx, store.InterviewPack{
		JobID: jobID, PromptID: prompt.ID, KnowledgeHash: knowledgeHash, Model: answer.Model, Pack: pack,
	})
}

// formatPackInput is what the model reads besides the knowledge base: the
// posting, and the market gaps it asks for.
func formatPackInput(job store.Job, gaps []store.MarketGap) string {
	var text strings.Builder
	fmt.Fprintf(&text, "## The posting\n\n%s\n\n## Market gaps this role asks for\n\n", jobfacts.FormatJobText(job, maximumDescriptionLength))
	var touched []string
	for _, gap := range gaps {
		if slices.Contains(gap.JobIDs, job.ID) {
			touched = append(touched, "- "+gap.Technology)
		}
	}
	if len(touched) == 0 {
		text.WriteString("None.\n")
	} else {
		text.WriteString(strings.Join(touched, "\n") + "\n")
	}
	return text.String()
}

type modelAnswer struct {
	Questions []struct {
		Question      string   `json:"question"`
		Reason        string   `json:"reason"`
		Stories       []string `json:"stories"`
		TalkingPoints string   `json:"talking_points"`
	} `json:"questions"`
	RoleGaps []struct {
		Gap          string `json:"gap"`
		HonestAnswer string `json:"honest_answer"`
	} `json:"role_gaps"`
}

// Pack is an interview pack as the hub keeps it: each question's stories
// are the entries the model cited, by id and title.
type Pack struct {
	Questions []PackQuestion `json:"questions"`
	RoleGaps  []PackRoleGap  `json:"role_gaps"`
}

type PackQuestion struct {
	Question      string      `json:"question"`
	Reason        string      `json:"reason"`
	Stories       []PackStory `json:"stories"`
	TalkingPoints string      `json:"talking_points"`
}

type PackStory struct {
	EntryID uuid.UUID `json:"entry_id"`
	Title   string    `json:"title"`
}

type PackRoleGap struct {
	Gap          string `json:"gap"`
	HonestAnswer string `json:"honest_answer"`
}

// convertAnswerToPack turns the model's references ("E3") into the entries
// they stand for, dropping any that name no confirmed entry.
func convertAnswerToPack(raw json.RawMessage, refs map[string]uuid.UUID, entries []store.ProfileEntry) (json.RawMessage, error) {
	var answer modelAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil, err
	}
	titles := map[uuid.UUID]string{}
	for _, entry := range entries {
		titles[entry.ID] = entry.Title
	}
	pack := Pack{Questions: []PackQuestion{}, RoleGaps: []PackRoleGap{}}
	for _, question := range answer.Questions {
		converted := PackQuestion{Question: question.Question, Reason: question.Reason, TalkingPoints: question.TalkingPoints, Stories: []PackStory{}}
		for _, ref := range question.Stories {
			if entryID, known := refs[strings.Trim(strings.TrimSpace(ref), "[]")]; known {
				converted.Stories = append(converted.Stories, PackStory{EntryID: entryID, Title: titles[entryID]})
			}
		}
		pack.Questions = append(pack.Questions, converted)
	}
	for _, gap := range answer.RoleGaps {
		pack.RoleGaps = append(pack.RoleGaps, PackRoleGap{Gap: gap.Gap, HonestAnswer: gap.HonestAnswer})
	}
	return json.Marshal(pack)
}
