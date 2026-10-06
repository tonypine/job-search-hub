// Package boardpoller keeps the jobs of the companies with a verified board
// current by reading their boards on an interval. A watched company's board
// stores every new posting; any other only the new postings that could fit.
package boardpoller

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type postingFetcher interface {
	FetchPostings(ctx context.Context, provider, boardToken string) ([]store.JobPosting, error)
}

// PollSummary counts one pass over the boards.
type PollSummary struct {
	Boards  int
	Skipped int
	Failed  int
	// Totals.Dropped counts the new postings of unwatched companies' boards
	// that couldn't fit, left unstored.
	Totals store.BoardSyncResult
}

type Poller struct {
	hub     *store.Store
	fetcher postingFetcher
	now     func() time.Time
}

func New(hub *store.Store, fetcher postingFetcher) *Poller {
	return &Poller{hub: hub, fetcher: fetcher, now: time.Now}
}

// Run polls once at start and then every interval, until ctx ends.
func (poller *Poller) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			summary, err := poller.PollOnce(ctx)
			if err != nil {
				slog.Error("board poll failed", "error", err)
			} else {
				slog.Info("board poll done", "boards", summary.Boards, "skipped", summary.Skipped, "failed", summary.Failed,
					"created", summary.Totals.Created, "adopted", summary.Totals.Adopted, "closed", summary.Totals.Closed,
					"reopened", summary.Totals.Reopened, "seen", summary.Totals.Seen, "dropped", summary.Totals.Dropped)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// discoveredPollInterval is how often a board discovered in bulk is read:
// there are thousands, and each lists the owner's roles rarely.
const discoveredPollInterval = 24 * time.Hour

// PollOnce syncs every verified board, a board discovered in bulk at most
// daily. A board that cannot be read is logged and counted, and the others
// go on.
func (poller *Poller) PollOnce(ctx context.Context) (PollSummary, error) {
	saved, err := poller.hub.GetJobCriteria(ctx)
	if err != nil {
		return PollSummary{}, err
	}
	boards, err := poller.hub.ListPolledJobBoards(ctx, jobboards.PostingProviders, poller.now().Add(-discoveredPollInterval))
	if err != nil {
		return PollSummary{}, err
	}
	summary := PollSummary{Boards: len(boards)}
	for _, board := range boards {
		postings, err := poller.fetcher.FetchPostings(ctx, board.Provider, board.BoardToken)
		if errors.Is(err, jobboards.ErrPostingAPIOff) {
			summary.Skipped++
			slog.Info("board skipped: its posting API is off", "provider", board.Provider, "board", board.BoardToken)
			continue
		}
		if err != nil {
			summary.Failed++
			slog.Warn("board sync failed", "provider", board.Provider, "board", board.BoardToken, "error", err)
			continue
		}
		var couldFit func(store.JobPosting) bool
		if !board.IsWatched {
			couldFit = func(posting store.JobPosting) bool { return jobfit.CouldFit(posting, saved.Criteria) }
		}
		result, err := poller.hub.SyncBoardJobsStoringNewIf(ctx, store.Actor{Kind: store.ActorSystem}, board.JobBoard, postings, poller.now(), couldFit)
		if err != nil {
			return summary, err
		}
		if err := poller.hub.MarkJobBoardPolled(ctx, board.ID, poller.now()); err != nil {
			return summary, err
		}
		summary.Totals.Created += result.Created
		summary.Totals.Adopted += result.Adopted
		summary.Totals.Closed += result.Closed
		summary.Totals.Reopened += result.Reopened
		summary.Totals.Seen += result.Seen
		summary.Totals.Dropped += result.Dropped
	}
	return summary, nil
}

// SyncBoard reads one board's open postings and stores them all as its jobs,
// as when the owner or an agent sets a company's board.
func (poller *Poller) SyncBoard(ctx context.Context, board store.JobBoard) (store.BoardSyncResult, error) {
	postings, err := poller.fetcher.FetchPostings(ctx, board.Provider, board.BoardToken)
	if err != nil {
		return store.BoardSyncResult{}, err
	}
	return poller.hub.SyncBoardJobs(ctx, store.Actor{Kind: store.ActorSystem}, board, postings, poller.now())
}
