// Package feedpoller gathers postings open to the owner from job feeds on an
// interval: Himalayas by the search terms in the job criteria, and Remote OK's
// newest postings that could fit.
package feedpoller

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumPostingAge keeps the feed to recent postings: the ones worth
	// applying to early.
	maximumPostingAge     = 14 * 24 * time.Hour
	maximumPagesPerSearch = 5
)

// ErrNoSearchCriteria means the criteria name no search terms or no home
// country, so there is nothing to search by.
var ErrNoSearchCriteria = errors.New("the job criteria have no search terms or no home country")

type postingSearcher interface {
	SearchHimalayas(ctx context.Context, search jobboards.HimalayasSearch) ([]store.JobPosting, error)
	FetchRemoteOKPostings(ctx context.Context, tag string) ([]store.JobPosting, error)
}

type Poller struct {
	hub      *store.Store
	searcher postingSearcher
	now      func() time.Time
}

func New(hub *store.Store, searcher postingSearcher) *Poller {
	return &Poller{hub: hub, searcher: searcher, now: time.Now}
}

// Run polls once at start and then every interval, until ctx ends.
func (poller *Poller) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		result, err := poller.PollOnce(ctx)
		if err != nil {
			slog.Error("feed poll failed", "feed", jobboards.Himalayas, "error", err)
		} else {
			slog.Info("feed poll done", "feed", jobboards.Himalayas, "created", result.Created, "closed", result.Closed,
				"reopened", result.Reopened, "seen", result.Seen, "listed on board", result.ListedOnBoard)
		}
		remoteOK, err := poller.PollRemoteOK(ctx)
		if err != nil {
			slog.Error("feed poll failed", "feed", jobboards.RemoteOK, "error", err)
		} else {
			slog.Info("feed poll done", "feed", jobboards.RemoteOK, "created", remoteOK.Created, "closed", remoteOK.Closed,
				"reopened", remoteOK.Reopened, "seen", remoteOK.Seen, "listed on board", remoteOK.ListedOnBoard, "dropped", remoteOK.Dropped)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// PollOnce searches the feed for each search term and stores what it finds.
// A search that fails is logged, and the others go on.
func (poller *Poller) PollOnce(ctx context.Context) (store.FeedSyncResult, error) {
	saved, err := poller.hub.GetJobCriteria(ctx)
	if err != nil {
		return store.FeedSyncResult{}, err
	}
	criteria := saved.Criteria
	if len(criteria.SearchTerms) == 0 || criteria.HomeCountry == "" {
		return store.FeedSyncResult{}, ErrNoSearchCriteria
	}

	now := poller.now()
	seen := map[string]bool{}
	var postings []store.JobPosting
	for _, term := range criteria.SearchTerms {
		found, err := poller.searcher.SearchHimalayas(ctx, jobboards.HimalayasSearch{
			Term: term, Country: criteria.HomeCountry, PublishedAfter: now.Add(-maximumPostingAge), MaximumPages: maximumPagesPerSearch,
		})
		if err != nil {
			slog.Warn("feed search failed", "feed", jobboards.Himalayas, "term", term, "error", err)
			continue
		}
		for _, posting := range found {
			if !seen[posting.ExternalID] {
				seen[posting.ExternalID] = true
				postings = append(postings, posting)
			}
		}
	}
	return poller.hub.SyncFeedJobs(ctx, store.Actor{Kind: store.ActorSystem}, store.JobSourceHimalayas, postings, now)
}

// RemoteOKPollResult is a Remote OK poll's sync, and how many postings the
// filter left out.
type RemoteOKPollResult struct {
	store.FeedSyncResult
	Dropped int
}

// PollRemoteOK reads Remote OK's newest postings under each search term, as
// a tag such as "react" or "full-stack", and stores the recent ones that
// could fit. A tag that fails is logged, and the others go on.
func (poller *Poller) PollRemoteOK(ctx context.Context) (RemoteOKPollResult, error) {
	saved, err := poller.hub.GetJobCriteria(ctx)
	if err != nil {
		return RemoteOKPollResult{}, err
	}
	if len(saved.Criteria.SearchTerms) == 0 {
		return RemoteOKPollResult{}, ErrNoSearchCriteria
	}
	seen := map[string]bool{}
	var postings []store.JobPosting
	for _, term := range saved.Criteria.SearchTerms {
		tag := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(term)), " ", "-")
		found, err := poller.searcher.FetchRemoteOKPostings(ctx, tag)
		if err != nil {
			slog.Warn("feed search failed", "feed", jobboards.RemoteOK, "tag", tag, "error", err)
			continue
		}
		for _, posting := range found {
			if !seen[posting.ExternalID] {
				seen[posting.ExternalID] = true
				postings = append(postings, posting)
			}
		}
	}
	publishedAfter := poller.now().Add(-maximumPostingAge)
	kept := slices.DeleteFunc(slices.Clone(postings), func(posting store.JobPosting) bool {
		isOld := posting.PublishedAt != nil && posting.PublishedAt.Before(publishedAfter)
		return isOld || !jobfit.CouldFit(posting, saved.Criteria)
	})
	result, err := poller.hub.SyncFeedJobs(ctx, store.Actor{Kind: store.ActorSystem}, store.JobSourceRemoteOK, kept, poller.now())
	return RemoteOKPollResult{FeedSyncResult: result, Dropped: len(postings) - len(kept)}, err
}
