package marketgaps

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
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// refreshInterval is how old the gaps get before they're computed again.
	refreshInterval = 24 * time.Hour
	jobsPageSize    = 500
	// maximumExampleTitles is how many asking jobs a gap names for the model.
	maximumExampleTitles = 3
	maximumProfileLength = 3000
	maximumAnswerTokens  = 3000
)

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

type Analyzer struct {
	hub    *store.Store
	client modelClient
	rates  jobfit.RateSource
}

func NewAnalyzer(hub *store.Store, client modelClient, rates jobfit.RateSource) *Analyzer {
	return &Analyzer{hub: hub, client: client, rates: rates}
}

// Run refreshes the gaps when they're missing or a day old, checking every
// interval.
func (analyzer *Analyzer) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			computedAt, err := analyzer.hub.GetMarketGapsComputedAt(ctx)
			if err == nil && (computedAt == nil || time.Since(*computedAt) >= refreshInterval) {
				if gaps, err := analyzer.Refresh(ctx); errors.Is(err, ErrNoPrompt) {
					slog.Info("market gaps wait for a market_gaps prompt")
				} else if err != nil {
					slog.Error("market gaps refresh failed", "error", err)
				} else {
					slog.Info("market gaps refreshed", "gaps", len(gaps))
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// fitJob is a good fit as the plans name it.
type fitJob struct {
	title   string
	company string
}

// ErrNoPrompt is a refresh asked for before any market_gaps prompt is saved.
var ErrNoPrompt = errors.New("no market_gaps prompt is saved yet")

// Refresh finds the gaps among the open good fits, asks the market_gaps
// model for a plan to close each, and saves them. A plan that fails leaves
// its gap without one.
func (analyzer *Analyzer) Refresh(ctx context.Context) ([]store.MarketGap, error) {
	prompt, err := analyzer.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindMarketGaps)
	if errors.Is(err, store.ErrAgentPromptNotFound) {
		return nil, ErrNoPrompt
	}
	if err != nil {
		return nil, err
	}
	goodFits, jobs, err := analyzer.listGoodFits(ctx)
	if err != nil {
		return nil, err
	}
	knowledgeBase, err := analyzer.readKnowledgeBase(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	found := FindGaps(goodFits, knowledgeBase)
	gaps := make([]store.MarketGap, len(found))
	for index, gap := range found {
		gaps[index] = store.MarketGap{Technology: gap.Technology, JobCount: len(gap.JobIDs), GoodFits: len(goodFits), JobIDs: gap.JobIDs, ComputedAt: now}
	}
	if len(gaps) > 0 {
		if err := analyzer.addPlans(ctx, prompt, gaps, jobs); err != nil {
			slog.Warn("market gap plans failed; the gaps are kept without them", "error", err)
		}
	}
	return gaps, analyzer.hub.SaveMarketGaps(ctx, gaps)
}

func (analyzer *Analyzer) listGoodFits(ctx context.Context) ([]GoodFit, map[uuid.UUID]fitJob, error) {
	criteria, rates, err := jobfit.ReadInputs(ctx, analyzer.hub, analyzer.rates)
	if err != nil {
		return nil, nil, err
	}
	var goodFits []GoodFit
	jobs := map[uuid.UUID]fitJob{}
	for offset := 0; ; offset += jobsPageSize {
		items, total, err := analyzer.hub.ListJobs(ctx, store.JobFilter{Status: store.JobStatusOpen, Limit: jobsPageSize, Offset: offset})
		if err != nil {
			return nil, nil, err
		}
		for _, item := range items {
			if jobfit.Judge(item.Job, item.Facts, criteria, rates).Level != jobfit.LevelGood {
				continue
			}
			var facts struct {
				Technologies []string `json:"technologies"`
			}
			if len(item.Facts) > 0 {
				_ = json.Unmarshal(item.Facts, &facts)
			}
			goodFits = append(goodFits, GoodFit{JobID: item.Job.ID, Technologies: facts.Technologies})
			job := fitJob{title: item.Job.Title}
			if item.CompanyName != nil {
				job.company = *item.CompanyName
			}
			jobs[item.Job.ID] = job
		}
		if offset+jobsPageSize >= total {
			return goodFits, jobs, nil
		}
	}
}

// readKnowledgeBase is everything that says what the owner knows: the
// knowledge-base entries, the base CV and the profile.
func (analyzer *Analyzer) readKnowledgeBase(ctx context.Context) (string, error) {
	var text strings.Builder
	entries, err := analyzer.hub.ListProfileEntries(ctx, store.ProfileEntryFilter{})
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		fmt.Fprintf(&text, "%s\n%s\n%s\n%s\n", entry.Title, entry.Body, entry.Organization, strings.Join(entry.Skills, ", "))
	}
	if base, err := analyzer.hub.GetBaseCV(ctx); err == nil {
		encoded, _ := json.Marshal(base.Content)
		text.Write(encoded)
	} else if !errors.Is(err, store.ErrCVNotFound) {
		return "", err
	}
	profile, err := analyzer.hub.GetOwnerProfile(ctx)
	if err != nil {
		return "", err
	}
	text.WriteString("\n" + profile.Body)
	return text.String(), nil
}

type plansAnswer struct {
	Plans []struct {
		Gap  string `json:"gap"`
		Kind string `json:"kind"`
		Plan string `json:"plan"`
	} `json:"plans"`
}

// addPlans asks the market_gaps model for a plan per gap, in one request,
// and sets each gap's plan from the answer.
func (analyzer *Analyzer) addPlans(ctx context.Context, prompt store.AgentPrompt, gaps []store.MarketGap, jobs map[uuid.UUID]fitJob) error {
	profile, err := analyzer.hub.GetOwnerProfile(ctx)
	if err != nil {
		return err
	}
	answer, err := analyzer.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		System: prompt.Body, User: formatPlansInput(gaps, jobs, profile.Body), SchemaName: store.AgentPromptKindMarketGaps,
		Schema: prompt.ResultSchema, Examples: prompt.Examples, MaxTokens: maximumAnswerTokens,
		Task: chatcompletions.TaskLabel{PromptID: &prompt.ID, PromptVersion: prompt.Version},
	})
	if err != nil {
		return err
	}
	var plans plansAnswer
	if err := json.Unmarshal(answer.Object, &plans); err != nil {
		return err
	}
	for _, plan := range plans.Plans {
		for index := range gaps {
			if canonicalize(gaps[index].Technology) == canonicalize(plan.Gap) {
				gaps[index].PlanKind, gaps[index].Plan = plan.Kind, strings.TrimSpace(plan.Plan)
			}
		}
	}
	return nil
}

// formatPlansInput is what the planner reads: the owner's profile, then each
// gap with how many good fits ask for it and a few of them.
func formatPlansInput(gaps []store.MarketGap, jobs map[uuid.UUID]fitJob, profile string) string {
	var text strings.Builder
	if len(profile) > maximumProfileLength {
		profile = strings.ToValidUTF8(profile[:maximumProfileLength], "")
	}
	fmt.Fprintf(&text, "## The candidate\n\n%s\n\n## The gaps\n\n", strings.TrimSpace(profile))
	for _, gap := range gaps {
		var examples []string
		for _, jobID := range gap.JobIDs[:min(len(gap.JobIDs), maximumExampleTitles)] {
			job := jobs[jobID]
			if job.company != "" {
				examples = append(examples, job.title+" at "+job.company)
			} else {
				examples = append(examples, job.title)
			}
		}
		fmt.Fprintf(&text, "- %s: asked for by %d of %d good fits, such as %s\n", gap.Technology, gap.JobCount, gap.GoodFits, strings.Join(examples, "; "))
	}
	return text.String()
}
