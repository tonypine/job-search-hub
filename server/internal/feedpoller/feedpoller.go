// Package feedpoller gathers postings open to the owner from job feeds, by
// the search terms in the job criteria, on an interval.
package feedpoller

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
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
			slog.Error("feed poll failed", "error", err)
		} else {
			slog.Info("feed poll done", "feed", jobboards.Himalayas, "created", result.Created, "closed", result.Closed,
				"reopened", result.Reopened, "seen", result.Seen, "listed on board", result.ListedOnBoard)
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
