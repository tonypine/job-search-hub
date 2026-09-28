// Package boardpoller keeps the jobs of watched companies current by reading
// their job boards on an interval.
package boardpoller

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type postingFetcher interface {
	FetchPostings(ctx context.Context, provider, boardToken string) ([]store.JobPosting, error)
}

// PollSummary counts one pass over the watched boards.
type PollSummary struct {
	Boards  int
	Skipped int
	Failed  int
	Totals  store.BoardSyncResult
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
		summary, err := poller.PollOnce(ctx)
		if err != nil {
			slog.Error("board poll failed", "error", err)
		} else {
			slog.Info("board poll done", "boards", summary.Boards, "skipped", summary.Skipped, "failed", summary.Failed,
				"created", summary.Totals.Created, "closed", summary.Totals.Closed, "reopened", summary.Totals.Reopened, "seen", summary.Totals.Seen)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// PollOnce syncs every watched board. A board that cannot be read is logged
// and counted, and the others go on.
func (poller *Poller) PollOnce(ctx context.Context) (PollSummary, error) {
	boards, err := poller.hub.ListWatchedJobBoards(ctx)
	if err != nil {
		return PollSummary{}, err
	}
	summary := PollSummary{Boards: len(boards)}
	for _, board := range boards {
		result, err := poller.SyncBoard(ctx, board)
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
		summary.Totals.Created += result.Created
		summary.Totals.Closed += result.Closed
		summary.Totals.Reopened += result.Reopened
		summary.Totals.Seen += result.Seen
	}
	return summary, nil
}

// SyncBoard reads one board's open postings and stores them as its jobs.
func (poller *Poller) SyncBoard(ctx context.Context, board store.JobBoard) (store.BoardSyncResult, error) {
	postings, err := poller.fetcher.FetchPostings(ctx, board.Provider, board.BoardToken)
	if err != nil {
		return store.BoardSyncResult{}, err
	}
	return poller.hub.SyncBoardJobs(ctx, store.Actor{Kind: store.ActorSystem}, board, postings, poller.now())
}
