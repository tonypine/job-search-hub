package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrJobBoardTaken = errors.New("this job board is already stored for another company")

// JobBoardProviders are the providers a job board can be stored under.
var JobBoardProviders = []string{
	"greenhouse", "lever", "ashby", "workable", "recruitee", "personio", "smartrecruiters", "eightfold", "bamboohr", "pinpoint", "other",
}

type JobBoard struct {
	ID uuid.UUID `json:"id"`
	// CompanyID is nil for a board found for a company the hub keeps no
	// record of; CompanyName names that company.
	CompanyID        *uuid.UUID `json:"company_id,omitempty"`
	CompanyName      string     `json:"company_name,omitempty"`
	Provider         string     `json:"provider"`
	BoardToken       string     `json:"board_token"`
	BoardURL         string     `json:"board_url,omitempty"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	OpenPostingCount *int       `json:"open_posting_count,omitempty"`
}

const jobBoardColumns = `id, company_id, company_name, provider, board_token, board_url, verified_at, open_posting_count`

func scanJobBoard(row pgx.Row) (JobBoard, error) {
	var board JobBoard
	err := row.Scan(&board.ID, &board.CompanyID, &board.CompanyName, &board.Provider, &board.BoardToken, &board.BoardURL, &board.VerifiedAt,
		&board.OpenPostingCount)
	return board, err
}

// JobBoardInput is a board to store. A verified board may still have a nil
// OpenPostingCount when its provider confirms the board but not its postings.
type JobBoardInput struct {
	CompanyID        uuid.UUID
	Provider         string
	BoardToken       string
	BoardURL         string
	Verified         bool
	OpenPostingCount *int
	SourceURL        string
}

// SetJobBoard stores a company's job board, or refreshes it when the company
// already has this board. A found board that belongs to no company of the
// hub's becomes this company's.
func (s *Store) SetJobBoard(ctx context.Context, actor Actor, input JobBoardInput) (JobBoard, error) {
	if !slices.Contains(JobBoardProviders, input.Provider) {
		return JobBoard{}, fmt.Errorf("provider must be one of %s", strings.Join(JobBoardProviders, ", "))
	}
	boardToken := strings.TrimSpace(input.BoardToken)
	if boardToken == "" {
		return JobBoard{}, errors.New("a job board needs a board token")
	}
	var verifiedAt *time.Time
	if input.Verified {
		now := time.Now()
		verifiedAt = &now
	}

	var board JobBoard
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		existing, err := scanJobBoard(tx.QueryRow(ctx, `
			SELECT `+jobBoardColumns+` FROM job_boards WHERE provider = $1 AND board_token = $2 FOR UPDATE`,
			input.Provider, boardToken))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			board, err = scanJobBoard(tx.QueryRow(ctx, `
				INSERT INTO job_boards (company_id, provider, board_token, board_url, verified_at, open_posting_count)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING `+jobBoardColumns,
				input.CompanyID, input.Provider, boardToken, input.BoardURL, verifiedAt, input.OpenPostingCount))
			if isForeignKeyViolation(err) {
				return ErrCompanyNotFound
			}
			if err != nil {
				return err
			}
			return insertChange(ctx, tx, actor, change{
				entityType: "job_board", entityID: board.ID, operation: "create", after: board, sourceURL: input.SourceURL,
			})
		case err != nil:
			return err
		case existing.CompanyID != nil && *existing.CompanyID != input.CompanyID:
			return ErrJobBoardTaken
		}

		board, err = scanJobBoard(tx.QueryRow(ctx, `
			UPDATE job_boards SET company_id = $5, board_url = $2, verified_at = $3, open_posting_count = $4, updated_at = now()
			WHERE id = $1
			RETURNING `+jobBoardColumns,
			existing.ID, input.BoardURL, verifiedAt, input.OpenPostingCount, input.CompanyID))
		if isForeignKeyViolation(err) {
			return ErrCompanyNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE jobs SET company_id = $2 WHERE job_board_id = $1 AND company_id IS NULL`, board.ID, input.CompanyID); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "job_board", entityID: board.ID, operation: "update", before: existing, after: board, sourceURL: input.SourceURL,
		})
	})
	return board, err
}

func (s *Store) ListJobBoards(ctx context.Context, companyID uuid.UUID) ([]JobBoard, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+jobBoardColumns+` FROM job_boards WHERE company_id = $1 ORDER BY created_at`, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobBoard, error) { return scanJobBoard(row) })
}

// ListWatchedJobBoards returns the verified boards of watched companies on the
// providers whose postings can be fetched.
func (s *Store) ListWatchedJobBoards(ctx context.Context) ([]JobBoard, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+jobBoardColumns+` FROM job_boards
		WHERE verified_at IS NOT NULL
		  AND provider IN ('greenhouse', 'lever', 'ashby')
		  AND company_id IN (SELECT company_id FROM watch_list_entries WHERE removed_at IS NULL)
		ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobBoard, error) { return scanJobBoard(row) })
}

var ErrJobBoardNotFound = errors.New("job board not found")

func (s *Store) GetJobBoardByToken(ctx context.Context, provider, boardToken string) (JobBoard, error) {
	board, err := scanJobBoard(s.pool.QueryRow(ctx, `SELECT `+jobBoardColumns+` FROM job_boards WHERE provider = $1 AND board_token = $2`, provider, boardToken))
	if errors.Is(err, pgx.ErrNoRows) {
		return JobBoard{}, ErrJobBoardNotFound
	}
	return board, err
}
