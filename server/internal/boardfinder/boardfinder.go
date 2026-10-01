// Package boardfinder finds the job boards of the companies behind good and
// unclear jobs from the feeds and mail alerts, so their postings can be read
// first-hand. A board counts as theirs only when it lists one of their feed
// jobs' titles: providers answer for any name, and names collide.
package boardfinder

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

const (
	// searchAgainAfter is how long a company searched on every provider
	// without a board waits before the next search.
	searchAgainAfter = 7 * 24 * time.Hour
	// maximumSearchesPerPass bounds one pass; the next pass picks up the rest.
	maximumSearchesPerPass = 50
	jobPageSize            = 500
)

// boardProviders are the providers searched, in order: those whose postings
// the hub reads.
var boardProviders = []string{jobboards.Greenhouse, jobboards.Lever, jobboards.Ashby, jobboards.Workable}

// slowProviders limit requests harder than the others, and get this pause
// before each request instead of RequestPause.
var slowProviders = map[string]time.Duration{jobboards.Workable: 2 * time.Second}

// feedSources are the sources whose jobs name a company without its board.
var feedSources = []string{store.JobSourceHimalayas, store.JobSourceIndeed, store.JobSourceLinkedIn, store.JobSourceGlassdoor}

type postingFetcher interface {
	FetchPostings(ctx context.Context, provider, boardToken string) ([]store.JobPosting, error)
}

type Finder struct {
	hub     *store.Store
	fetcher postingFetcher
	rates   jobfit.RateSource
	// RequestPause spaces the requests to the providers.
	RequestPause time.Duration
	now          func() time.Time
	// limitingProviders are the providers that answered 429 this pass; they
	// aren't asked again until the next one.
	limitingProviders map[string]bool
}

func New(hub *store.Store, fetcher postingFetcher, rates jobfit.RateSource) *Finder {
	return &Finder{hub: hub, fetcher: fetcher, rates: rates, RequestPause: 500 * time.Millisecond, now: time.Now}
}

// PassSummary counts one pass over the companies awaiting a search.
type PassSummary struct {
	// Searched companies have no board on any provider.
	Searched int
	Found    int
	// Unfinished companies have providers left to ask, as when one failed or
	// was limiting requests; the next pass asks those.
	Unfinished int
}

// Run searches once at start and then every interval, until ctx ends.
func (finder *Finder) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		summary, err := finder.FindOnce(ctx)
		if err != nil {
			slog.Error("board search pass stopped", "error", err)
		} else if summary != (PassSummary{}) {
			slog.Info("board search pass done", "searched", summary.Searched, "found", summary.Found, "unfinished", summary.Unfinished)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// FeedCompany is a company that good or unclear feed jobs name.
type FeedCompany struct {
	Name   string
	Titles []string
	// HimalayasSlug is the company's name in its Himalayas job URLs.
	HimalayasSlug string
	HasGoodFit    bool
	// SearchedProviders answered without a board in this round of searches;
	// ProvidersToSearch are the rest.
	SearchedProviders []string
	ProvidersToSearch []string
}

// FindOnce searches the boards of the companies that have none stored and
// providers left to ask, good fits first. Each company's search asks the
// providers it hasn't yet, and records the ones that answered.
func (finder *Finder) FindOnce(ctx context.Context) (PassSummary, error) {
	companies, err := finder.listCompaniesToSearch(ctx)
	if err != nil {
		return PassSummary{}, err
	}
	var summary PassSummary
	finder.limitingProviders = map[string]bool{}
	for _, company := range companies[:min(len(companies), maximumSearchesPerPass)] {
		board, found, answered, err := finder.searchBoard(ctx, company)
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		searchedProviders := append(slices.Clone(company.SearchedProviders), answered...)
		var boardID *uuid.UUID
		switch {
		case found:
			saved, err := finder.hub.SaveFoundJobBoard(ctx, store.Actor{Kind: store.ActorSystem}, board)
			if err != nil {
				return summary, err
			}
			boardID = &saved.ID
			summary.Found++
			slog.Info("board found", "company", company.Name, "provider", board.Provider, "board", board.BoardToken)
		case len(answered) == len(company.ProvidersToSearch):
			summary.Searched++
		default:
			summary.Unfinished++
			slog.Info("board search unfinished", "company", company.Name, "error", err)
		}
		if err := finder.hub.RecordBoardSearch(ctx, company.Name, searchedProviders, boardID, finder.now()); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

// listCompaniesToSearch returns the companies behind open good or unclear
// feed jobs that have no board stored and providers left to ask, good fits
// first, then the ones with the most jobs.
func (finder *Finder) listCompaniesToSearch(ctx context.Context) ([]FeedCompany, error) {
	criteria, rates, err := jobfit.ReadInputs(ctx, finder.hub, finder.rates)
	if err != nil {
		return nil, err
	}
	withBoards, err := finder.hub.ListCompanyNamesWithJobBoards(ctx)
	if err != nil {
		return nil, err
	}
	searches, err := finder.hub.ListBoardSearches(ctx)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*FeedCompany{}
	var keys []string
	for offset := 0; ; offset += jobPageSize {
		items, _, err := finder.hub.ListJobs(ctx, store.JobFilter{Status: store.JobStatusOpen, Limit: jobPageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			name := getCompanyName(item)
			key := store.NormalizeCompanyName(name)
			if key == "" || !slices.Contains(feedSources, item.Job.Source) || withBoards[key] {
				continue
			}
			company, known := byKey[key]
			if !known {
				searchedProviders := getSearchedProviders(searches, key, finder.now())
				providersToSearch := slices.DeleteFunc(slices.Clone(boardProviders), func(provider string) bool {
					return slices.Contains(searchedProviders, provider)
				})
				if len(providersToSearch) == 0 {
					continue
				}
				company = &FeedCompany{Name: name, SearchedProviders: searchedProviders, ProvidersToSearch: providersToSearch}
			}
			level := jobfit.Judge(item.Job, item.Facts, criteria, rates).Level
			if level == jobfit.LevelPoor {
				continue
			}
			if !known {
				byKey[key] = company
				keys = append(keys, key)
			}
			company.Titles = append(company.Titles, item.Job.Title)
			company.HasGoodFit = company.HasGoodFit || level == jobfit.LevelGood
			if slug := getHimalayasCompanySlug(item.Job.URL); slug != "" {
				company.HimalayasSlug = slug
			}
		}
		if len(items) < jobPageSize {
			break
		}
	}
	companies := make([]FeedCompany, 0, len(keys))
	for _, key := range keys {
		companies = append(companies, *byKey[key])
	}
	slices.SortStableFunc(companies, func(a, b FeedCompany) int {
		if a.HasGoodFit != b.HasGoodFit {
			if a.HasGoodFit {
				return -1
			}
			return 1
		}
		return cmp.Compare(len(b.Titles), len(a.Titles))
	})
	return companies, nil
}

// getSearchedProviders returns the providers that answered the company's
// current round of searches; a round older than searchAgainAfter is over.
func getSearchedProviders(searches map[string]store.BoardSearch, key string, now time.Time) []string {
	search, found := searches[key]
	if !found || now.Sub(search.SearchedAt) > searchAgainAfter {
		return nil
	}
	return search.SearchedProviders
}

// searchBoard asks each provider left for the company, under each board token
// it may use, and returns the first board listing one of the company's
// titles. answered are the providers that answered every token without a
// board; err is the last failure, when a provider couldn't answer.
func (finder *Finder) searchBoard(ctx context.Context, company FeedCompany) (store.FoundJobBoardInput, bool, []string, error) {
	var answered []string
	var failure error
	for _, provider := range company.ProvidersToSearch {
		isAnswered := !finder.limitingProviders[provider]
		if !isAnswered {
			failure = jobboards.ErrRateLimited
		}
		for _, boardToken := range getBoardTokenCandidates(company) {
			if !isAnswered {
				break
			}
			if err := finder.waitBeforeRequest(ctx, provider); err != nil {
				return store.FoundJobBoardInput{}, false, answered, err
			}
			postings, err := finder.fetcher.FetchPostings(ctx, provider, boardToken)
			switch {
			case errors.Is(err, jobboards.ErrPostingAPIOff):
				continue
			case errors.Is(err, jobboards.ErrRateLimited):
				finder.limitingProviders[provider] = true
				isAnswered, failure = false, err
				continue
			case err != nil:
				isAnswered, failure = false, err
				continue
			}
			if hasAnyTitle(postings, company.Titles) {
				return store.FoundJobBoardInput{
					CompanyName: company.Name, Provider: provider, BoardToken: boardToken,
					BoardURL: jobboards.GetBoardURL(provider, boardToken), OpenPostingCount: len(postings),
				}, true, answered, nil
			}
		}
		if isAnswered {
			answered = append(answered, provider)
		}
	}
	return store.FoundJobBoardInput{}, false, answered, failure
}

func (finder *Finder) waitBeforeRequest(ctx context.Context, provider string) error {
	pause, isSlow := slowProviders[provider]
	if !isSlow || finder.RequestPause == 0 {
		pause = finder.RequestPause
	}
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// getCompanyName returns the hub company's name, or the name the feed gave.
func getCompanyName(item store.JobListItem) string {
	if item.CompanyName == nil {
		return ""
	}
	return *item.CompanyName
}

var himalayasCompanySlug = regexp.MustCompile(`^https://himalayas\.app/companies/([a-z0-9-]+)/jobs/`)

func getHimalayasCompanySlug(jobURL string) string {
	if match := himalayasCompanySlug.FindStringSubmatch(jobURL); match != nil {
		return match[1]
	}
	return ""
}

var nonTokenCharacters = regexp.MustCompile(`[^a-z0-9]+`)

// getBoardTokenCandidates returns the board tokens the company likely uses:
// its name joined and hyphenated, its Himalayas slug, and its first word, as
// "teravision" for Teravision Technologies.
func getBoardTokenCandidates(company FeedCompany) []string {
	words := strings.Fields(nonTokenCharacters.ReplaceAllString(store.NormalizeCompanyName(company.Name), " "))
	if len(words) == 0 {
		return nil
	}
	var candidates []string
	for _, candidate := range []string{strings.Join(words, ""), strings.Join(words, "-"), company.HimalayasSlug, words[0]} {
		if candidate != "" && !slices.Contains(candidates, candidate) {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func hasAnyTitle(postings []store.JobPosting, titles []string) bool {
	wanted := map[string]bool{}
	for _, title := range titles {
		wanted[wordmatch.Normalize(title)] = true
	}
	return slices.ContainsFunc(postings, func(posting store.JobPosting) bool { return wanted[wordmatch.Normalize(posting.Title)] })
}
