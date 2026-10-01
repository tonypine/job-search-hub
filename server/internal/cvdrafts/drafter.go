// Package cvdrafts has Claude tailor the owner's base CV to a pursued or
// good-fit job, and has each tailored CV printed to its PDF file.
// Every bullet of a draft cites its source, a bullet of the base CV or a
// confirmed knowledge-base entry; a draft citing anything else is refused.
package cvdrafts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
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

// maximumGeneratedPerPass bounds the CVs one pass drafts; the next pass
// takes the rest, newest first, so new postings get theirs first.
// maximumPrintedPerPass bounds the reprints of edited CVs.
const (
	maximumGeneratedPerPass = 3
	maximumPrintedPerPass   = 10
	jobsPageSize            = 500
)

type cvPrinter interface {
	PrintCV(ctx context.Context, actor store.Actor, cvID uuid.UUID) (store.CV, error)
}

// Drafter drafts tailored CVs through client, Claude, and prints them with
// Printer when there is one. Rates judge foreign pay in the fit that picks
// which jobs get a CV on their own.
type Drafter struct {
	hub     *store.Store
	client  modelClient
	Printer cvPrinter
	Rates   jobfit.RateSource
	// drafting serializes drafts, so a pass and a run for older postings
	// never draft the same job twice.
	drafting     sync.Mutex
	isGenerating atomic.Bool
}

func NewDrafter(hub *store.Store, client modelClient) *Drafter {
	return &Drafter{hub: hub, client: client}
}

// Run generates CVs at start and then every interval, until ctx ends: up to
// maximumGeneratedPerPass jobs that need one, and a print of each edited CV.
func (drafter *Drafter) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		jobIDs, err := drafter.ListJobsNeedingCV(ctx)
		if err != nil {
			slog.Error("list jobs needing a CV", "error", err)
		}
		for _, jobID := range jobIDs[:min(len(jobIDs), maximumGeneratedPerPass)] {
			drafter.generateAndLog(ctx, jobID)
		}
		drafter.printEditedCVs(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ListJobsNeedingCV returns the open jobs with no tailored CV that should
// have one: the pursued ones, then the good fits, the newest first.
func (drafter *Drafter) ListJobsNeedingCV(ctx context.Context) ([]uuid.UUID, error) {
	pursued, err := drafter.hub.ListJobsAwaitingCV(ctx)
	if err != nil {
		return nil, err
	}
	withCV, err := drafter.hub.ListTailoredCVJobIDs(ctx)
	if err != nil {
		return nil, err
	}
	criteria, rates, err := jobfit.ReadInputs(ctx, drafter.hub, drafter.getRates())
	if err != nil {
		return nil, err
	}
	jobIDs := slices.Clone(pursued)
	for offset := 0; ; offset += jobsPageSize {
		items, total, err := drafter.hub.ListJobs(ctx, store.JobFilter{Status: store.JobStatusOpen, Limit: jobsPageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if slices.Contains(withCV, item.Job.ID) || slices.Contains(jobIDs, item.Job.ID) {
				continue
			}
			if jobfit.Judge(item.Job, item.Facts, criteria, rates).Level == jobfit.LevelGood {
				jobIDs = append(jobIDs, item.Job.ID)
			}
		}
		if offset+jobsPageSize >= total {
			return jobIDs, nil
		}
	}
}

// GenerateCV makes sure the job has a printed tailored CV: it drafts one if
// the job has none, and prints it if it isn't printed. It returns the CV,
// with its PDF's path once printed.
func (drafter *Drafter) GenerateCV(ctx context.Context, jobID uuid.UUID) (store.CV, error) {
	drafter.drafting.Lock()
	defer drafter.drafting.Unlock()
	cv, err := drafter.hub.GetJobCV(ctx, jobID)
	if errors.Is(err, store.ErrCVNotFound) {
		cv, err = drafter.DraftCV(ctx, jobID)
	}
	if err != nil || cv.HasPDF || drafter.Printer == nil {
		return cv, err
	}
	return drafter.Printer.PrintCV(ctx, store.Actor{Kind: store.ActorSystem}, cv.ID)
}

// GenerateMissingCVs starts generating the CV of every job that needs one,
// in the background, and returns how many it will make. While a run is
// going, it starts no other and returns 0.
func (drafter *Drafter) GenerateMissingCVs(ctx context.Context) (int, error) {
	jobIDs, err := drafter.ListJobsNeedingCV(ctx)
	if err != nil || len(jobIDs) == 0 || !drafter.isGenerating.CompareAndSwap(false, true) {
		return 0, err
	}
	go func() {
		defer drafter.isGenerating.Store(false)
		runCtx := context.WithoutCancel(ctx)
		for _, jobID := range jobIDs {
			drafter.generateAndLog(runCtx, jobID)
		}
		slog.Info("missing CVs generated", "jobs", len(jobIDs))
	}()
	return len(jobIDs), nil
}

func (drafter *Drafter) generateAndLog(ctx context.Context, jobID uuid.UUID) {
	if cv, err := drafter.GenerateCV(ctx, jobID); err != nil {
		slog.Warn("CV generation failed", "job", jobID, "error", err)
	} else {
		slog.Info("CV generated", "job", jobID, "pdf", cv.PDFPath)
	}
}

// printEditedCVs prints the tailored CVs changed since their last print.
func (drafter *Drafter) printEditedCVs(ctx context.Context) {
	if drafter.Printer == nil {
		return
	}
	cvs, err := drafter.hub.ListTailoredCVsWithoutPDF(ctx, maximumPrintedPerPass)
	if err != nil {
		slog.Error("list CVs to print", "error", err)
		return
	}
	for _, cv := range cvs {
		if _, err := drafter.Printer.PrintCV(ctx, store.Actor{Kind: store.ActorSystem}, cv.ID); err != nil {
			slog.Warn("CV print failed", "cv", cv.ID, "error", err)
		}
	}
}

// getRates is the rate source the fit judges foreign pay with; without one,
// foreign pay reads unclear.
func (drafter *Drafter) getRates() jobfit.RateSource {
	if drafter.Rates == nil {
		return noRates{}
	}
	return drafter.Rates
}

type noRates struct{}

func (noRates) GetRates(context.Context, string) (map[string]float64, error) { return nil, nil }

// Draft is a tailored CV's words: as the job_cv schema asks Claude for them,
// and as the owner edits them.
type Draft struct {
	Label   string      `json:"label"`
	Summary string      `json:"summary"`
	Roles   []DraftRole `json:"roles"`
}

type DraftRole struct {
	Role    int           `json:"role"`
	Bullets []DraftBullet `json:"bullets"`
}

type DraftBullet struct {
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
	var draft Draft
	if err := json.Unmarshal(answer.Object, &draft); err != nil {
		return store.CV{}, fmt.Errorf("read the draft: %w", err)
	}
	if err := checkCitations(draft, base.Content, entries); err != nil {
		return store.CV{}, err
	}
	content, citations := assembleTailoredCV(base.Content, draft)
	return drafter.hub.SaveTailoredCV(ctx, store.Actor{Kind: store.ActorSystem}, jobID, content, citations)
}

// SaveEdit saves the owner's edit of the job's tailored CV: its headline,
// summary and bullets, each bullet still citing its source. It's rebuilt from
// the base CV, so the dates, companies and education stay the base CV's.
func (drafter *Drafter) SaveEdit(ctx context.Context, actor store.Actor, jobID uuid.UUID, edit Draft) (store.CV, error) {
	if _, err := drafter.hub.GetJobCV(ctx, jobID); err != nil {
		return store.CV{}, err
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
	if err := checkCitations(edit, base.Content, entries); err != nil {
		return store.CV{}, err
	}
	content, citations := assembleTailoredCV(base.Content, edit)
	return drafter.hub.SaveTailoredCV(ctx, actor, jobID, content, citations)
}

// checkCitations refuses a draft with a bullet whose source isn't a bullet
// of the base CV or a confirmed entry, or a role the base CV doesn't have.
func checkCitations(draft Draft, base resume.Resume, confirmed []store.ProfileEntry) error {
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
func assembleTailoredCV(base resume.Resume, draft Draft) (resume.Resume, map[string]string) {
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
