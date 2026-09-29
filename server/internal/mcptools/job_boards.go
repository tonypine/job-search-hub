package mcptools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type jobBoardVerifier interface {
	Verify(ctx context.Context, provider, boardToken string) (jobboards.Verification, error)
}

type jobBoardSyncer interface {
	SyncBoard(ctx context.Context, board store.JobBoard) (store.BoardSyncResult, error)
}

// boardSyncTimeout bounds the first read of a board just stored.
const boardSyncTimeout = time.Minute

type setJobBoardInput struct {
	CompanyID  uuid.UUID `json:"company_id"`
	Provider   string    `json:"provider" jsonschema:"one of greenhouse, lever, ashby, workable, recruitee, personio, smartrecruiters, other"`
	BoardToken string    `json:"board_token" jsonschema:"the board's identifier in the provider's job board URL, e.g. stripe in job-boards.greenhouse.io/stripe, jobs.lever.co/stripe or jobs.ashbyhq.com/stripe"`
	BoardURL   string    `json:"board_url,omitempty" jsonschema:"the public job board URL, for providers the hub cannot verify"`
	SourceURL  string    `json:"source_url,omitempty" jsonschema:"the page that led to this board, usually the careers page"`
}

func addJobBoardTools(server *mcp.Server, hub *store.Store, verifier jobBoardVerifier, syncer jobBoardSyncer) {
	addTool(server, &mcp.Tool{
		Name: "set_job_board",
		Description: "Store the job board where a company lists its open roles. Greenhouse, Lever, Ashby and Workable boards are " +
			"checked against the provider's public API first, and a board the provider does not know is rejected; " +
			"a verified board's jobs are read at once. Boards on other providers are stored unverified.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input setJobBoardInput) (*mcp.CallToolResult, store.JobBoard, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, store.JobBoard{}, err
		}

		board := store.JobBoardInput{
			CompanyID: input.CompanyID, Provider: input.Provider, BoardToken: input.BoardToken,
			BoardURL: input.BoardURL, SourceURL: input.SourceURL,
		}
		verification, err := verifier.Verify(ctx, input.Provider, input.BoardToken)
		switch {
		case errors.Is(err, jobboards.ErrUnsupportedProvider):
		case err != nil:
			return nil, store.JobBoard{}, err
		case !verification.Verified:
			return nil, store.JobBoard{}, fmt.Errorf("%s has no job board %q; check the token in the careers page's links", input.Provider, input.BoardToken)
		default:
			board.Verified = true
			board.BoardURL = verification.BoardURL
			board.OpenPostingCount = verification.OpenPostingCount
		}

		stored, err := hub.SetJobBoard(ctx, actor, board)
		if err == nil && board.Verified {
			go syncNewBoard(context.WithoutCancel(ctx), syncer, stored)
		}
		return nil, stored, err
	})
}

// syncNewBoard reads a board's jobs as soon as it is stored, rather than at
// the next poll, so a company just added shows its openings.
func syncNewBoard(ctx context.Context, syncer jobBoardSyncer, board store.JobBoard) {
	ctx, cancel := context.WithTimeout(ctx, boardSyncTimeout)
	defer cancel()
	result, err := syncer.SyncBoard(ctx, board)
	if err != nil {
		slog.Warn("new board not read", "provider", board.Provider, "board", board.BoardToken, "error", err)
		return
	}
	slog.Info("new board read", "provider", board.Provider, "board", board.BoardToken, "created", result.Created)
}
