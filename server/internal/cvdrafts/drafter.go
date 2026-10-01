// Package cvdrafts has Claude tailor the owner's base CV to a pursued job.
// Every bullet of a draft cites its source, a bullet of the base CV or a
// confirmed knowledge-base entry; a draft citing anything else is refused.
package cvdrafts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	maximumDescriptionLength = 20_000
	maximumAnswerTokens      = 4000
	baseSourcePrefix         = "base:"
	entrySourcePrefix        = "entry:"
)

// ErrNoCVDrafts says the hub has no way to draft CVs.
var ErrNoCVDrafts = errors.New("CV drafts are off: no Claude CLI is set up")

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

// Drafter drafts tailored CVs through client, Claude.
type Drafter struct {
	hub    *store.Store
	client modelClient
}

func NewDrafter(hub *store.Store, client modelClient) *Drafter {
	return &Drafter{hub: hub, client: client}
}

// Run drafts a CV for every pursued job without one, at start and then every
// interval, until ctx ends.
func (drafter *Drafter) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		jobIDs, err := drafter.hub.ListJobsAwaitingCV(ctx)
		if err != nil {
			slog.Error("list jobs awaiting a CV", "error", err)
		}
		for _, jobID := range jobIDs {
			if _, err := drafter.DraftCV(ctx, jobID); err != nil {
				slog.Warn("CV draft failed", "job", jobID, "error", err)
			} else {
				slog.Info("CV drafted", "job", jobID)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// draftAnswer is the tailored CV as the job_cv schema asks Claude for it.
type draftAnswer struct {
	Label   string       `json:"label"`
	Summary string       `json:"summary"`
	Roles   []roleAnswer `json:"roles"`
}

type roleAnswer struct {
	Role    int            `json:"role"`
	Bullets []bulletAnswer `json:"bullets"`
}

type bulletAnswer struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

// DraftCV has Claude tailor the base CV to the job and saves the draft as
// the job's tailored CV, once every bullet's citation checks out.
func (drafter *Drafter) DraftCV(ctx context.Context, jobID uuid.UUID) (store.CV, error) {
	if drafter.client == nil {
		return store.CV{}, ErrNoCVDrafts
	}
	prompt, err := drafter.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobCV)
	if err != nil {
		return store.CV{}, fmt.Errorf("read the job_cv prompt: %w", err)
	}
	base, err := drafter.hub.GetBaseCV(ctx)
	if err != nil {
		return store.CV{}, fmt.Errorf("read the base CV: %w", err)
	}
	confirmed := true
	entries, err := drafter.hub.ListProfileEntries(ctx, store.ProfileEntryFilter{Confirmed: &confirmed})
	if err != nil {
		return store.CV{}, err
	}
	details, err := drafter.hub.GetJobDetails(ctx, jobID)
	if err != nil {
		return store.CV{}, err
	}
	answer, err := drafter.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		System: prompt.Body, User: formatDraftInput(details, base.Content, entries),
		SchemaName: store.AgentPromptKindJobCV, Schema: prompt.ResultSchema, MaxTokens: maximumAnswerTokens,
		Task: chatcompletions.TaskLabel{SubjectID: &jobID, PromptID: &prompt.ID, PromptVersion: prompt.Version},
	})
	if err != nil {
		return store.CV{}, err
	}
	var draft draftAnswer
	if err := json.Unmarshal(answer.Object, &draft); err != nil {
		return store.CV{}, fmt.Errorf("read the draft: %w", err)
	}
	if err := checkCitations(draft, base.Content, entries); err != nil {
		return store.CV{}, err
	}
	content, citations := assembleTailoredCV(base.Content, draft)
	return drafter.hub.SaveTailoredCV(ctx, store.Actor{Kind: store.ActorSystem}, jobID, content, citations)
}

// checkCitations refuses a draft with a bullet whose source isn't a bullet
// of the base CV or a confirmed entry, or a role the base CV doesn't have.
func checkCitations(draft draftAnswer, base resume.Resume, confirmed []store.ProfileEntry) error {
	sources := map[string]bool{}
	for workIndex, work := range base.Work {
		for highlightIndex := range work.Highlights {
			sources[baseSourcePrefix+resume.GetBulletID(workIndex, highlightIndex)] = true
		}
	}
	for _, entry := range confirmed {
		sources[entrySourcePrefix+entry.ID.String()] = true
	}
	for _, role := range draft.Roles {
		if role.Role < 0 || role.Role >= len(base.Work) {
			return fmt.Errorf("%w: the draft names role %d, which the base CV doesn't have", chatcompletions.ErrInvalidAnswer, role.Role)
		}
		for _, bullet := range role.Bullets {
			if !sources[strings.TrimSpace(bullet.Source)] {
				return fmt.Errorf("%w: %q cites %q, which is neither a base CV bullet nor a confirmed entry", chatcompletions.ErrInvalidAnswer, bullet.Text, bullet.Source)
			}
		}
	}
	return nil
}

// assembleTailoredCV is the base CV with the draft's headline, summary and
// bullets: the roles, dates, education and skills stay the base CV's. A role
// the draft leaves out keeps its base bullets.
func assembleTailoredCV(base resume.Resume, draft draftAnswer) (resume.Resume, map[string]string) {
	tailored := base
	tailored.Work = append([]resume.Work(nil), base.Work...)
	if label := strings.TrimSpace(draft.Label); label != "" {
		tailored.Basics.Label = label
	}
	if summary := strings.TrimSpace(draft.Summary); summary != "" {
		tailored.Basics.Summary = summary
	}
	citations := map[string]string{}
	for workIndex, work := range base.Work {
		for highlightIndex := range work.Highlights {
			id := resume.GetBulletID(workIndex, highlightIndex)
			citations[id] = baseSourcePrefix + id
		}
	}
	for _, role := range draft.Roles {
		for highlightIndex := range tailored.Work[role.Role].Highlights {
			delete(citations, resume.GetBulletID(role.Role, highlightIndex))
		}
		highlights := make([]string, 0, len(role.Bullets))
		for _, bullet := range role.Bullets {
			citations[resume.GetBulletID(role.Role, len(highlights))] = strings.TrimSpace(bullet.Source)
			highlights = append(highlights, strings.TrimSpace(bullet.Text))
		}
		tailored.Work[role.Role].Highlights = highlights
	}
	return tailored, citations
}

// formatDraftInput is what Claude tailors from: the job, its brief, the base
// CV with each bullet's source, and the confirmed entries it may add.
func formatDraftInput(details store.JobDetails, base resume.Resume, confirmed []store.ProfileEntry) string {
	var text strings.Builder
	text.WriteString("## The job\n")
	if details.CompanyName != nil {
		fmt.Fprintf(&text, "Company: %s\n", *details.CompanyName)
	}
	text.WriteString(jobfacts.FormatJobText(details.Job, maximumDescriptionLength))
	if brief := details.Brief; brief != nil {
		fmt.Fprintf(&text, "\n\n## The job's brief\nMatch: %s. %s\n", brief.Match, brief.Reason)
		for _, point := range brief.Strengths {
			fmt.Fprintf(&text, "Strength: %s\n", point.Point)
		}
		for _, point := range brief.Weaknesses {
			fmt.Fprintf(&text, "Weakness: %s\n", point.Point)
		}
	}
	fmt.Fprintf(&text, "\n## The base CV\nHeadline: %s\nSummary: %s\nRoles, oldest last:\n", base.Basics.Label, base.Basics.Summary)
	for workIndex, work := range base.Work {
		fmt.Fprintf(&text, "Role %d: %s at %s (%s to %s). Skills: %s\n", workIndex, work.Position, work.Name, work.StartDate,
			formatEndDate(work.EndDate), strings.Join(work.Skills, ", "))
		for highlightIndex, highlight := range work.Highlights {
			fmt.Fprintf(&text, "  [%s%s] %s\n", baseSourcePrefix, resume.GetBulletID(workIndex, highlightIndex), highlight)
		}
	}
	text.WriteString("\n## Confirmed knowledge-base entries\n")
	if len(confirmed) == 0 {
		text.WriteString("None are confirmed yet: use only the base CV's bullets.\n")
	}
	for _, entry := range confirmed {
		fmt.Fprintf(&text, "[%s%s] %s: %s", entrySourcePrefix, entry.ID, entry.Kind, entry.Title)
		if entry.Organization != "" {
			fmt.Fprintf(&text, " at %s", entry.Organization)
		}
		if entry.Body != "" {
			fmt.Fprintf(&text, ". %s", strings.Join(strings.Fields(entry.Body), " "))
		}
		if entry.Outcome != "" {
			fmt.Fprintf(&text, ". Outcome: %s", entry.Outcome)
		}
		text.WriteString("\n")
	}
	return text.String()
}

func formatEndDate(endDate string) string {
	if endDate == "" {
		return "present"
	}
	return endDate
}
