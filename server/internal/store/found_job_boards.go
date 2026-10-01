package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FoundJobBoardInput is a verified board found for a company that feed jobs
// name.
type FoundJobBoardInput struct {
	CompanyName      string
	Provider         string
	BoardToken       string
	BoardURL         string
	OpenPostingCount int
	SourceURL        string
}

// SaveFoundJobBoard stores a board found for a company, tied to the hub's
// company of that name when there is one. A board already stored is returned
// as it is.
func (s *Store) SaveFoundJobBoard(ctx context.Context, actor Actor, input FoundJobBoardInput) (JobBoard, error) {
	if !slices.Contains(JobBoardProviders, input.Provider) {
		return JobBoard{}, errors.New("provider must be one of " + strings.Join(JobBoardProviders, ", "))
	}
	companyName := strings.TrimSpace(input.CompanyName)
	boardToken := strings.TrimSpace(input.BoardToken)
	if companyName == "" || boardToken == "" {
		return JobBoard{}, errors.New("a found board needs its company name and a board token")
	}
	var board JobBoard
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		companyIDs, err := getCompanyIDsByName(ctx, tx)
		if err != nil {
			return err
		}
		var companyID *uuid.UUID
		if id, found := companyIDs[NormalizeCompanyName(companyName)]; found {
			companyID = &id
		}
		openPostingCount := input.OpenPostingCount
		board, err = scanJobBoard(tx.QueryRow(ctx, `
			INSERT INTO job_boards (company_id, company_name, provider, board_token, board_url, verified_at, open_posting_count)
			VALUES ($1, $2, $3, $4, $5, now(), $6)
			ON CONFLICT (provider, board_token) DO NOTHING
			RETURNING `+jobBoardColumns,
			companyID, companyName, input.Provider, boardToken, input.BoardURL, &openPostingCount))
		if errors.Is(err, pgx.ErrNoRows) {
			board, err = scanJobBoard(tx.QueryRow(ctx, `SELECT `+jobBoardColumns+` FROM job_boards WHERE provider = $1 AND board_token = $2`,
				input.Provider, boardToken))
			return err
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "job_board", entityID: board.ID, operation: "create", after: board, sourceURL: input.SourceURL,
		})
	})
	return board, err
}

// ListCompanyNamesWithJobBoards returns the normalized names of the companies
// with a stored board, whether the hub keeps the company or not.
func (s *Store) ListCompanyNamesWithJobBoards(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT coalesce(companies.name, job_boards.company_name)
		FROM job_boards LEFT JOIN companies ON companies.id = job_boards.company_id`)
	if err != nil {
		return nil, err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	companies := make(map[string]bool, len(names))
	for _, name := range names {
		companies[NormalizeCompanyName(name)] = true
	}
	return companies, nil
}

// BoardSearch is how far the search for a company's board went.
type BoardSearch struct {
	SearchedAt time.Time
	// SearchedProviders answered without a board for the company.
	SearchedProviders []string
	JobBoardID        *uuid.UUID
}

// ListBoardSearches returns the searches made, by normalized company name.
func (s *Store) ListBoardSearches(ctx context.Context) (map[string]BoardSearch, error) {
	rows, err := s.pool.Query(ctx, `SELECT company_key, searched_at, searched_providers, job_board_id FROM board_searches`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	searches := map[string]BoardSearch{}
	for rows.Next() {
		var key string
		var search BoardSearch
		if err := rows.Scan(&key, &search.SearchedAt, &search.SearchedProviders, &search.JobBoardID); err != nil {
			return nil, err
		}
		searches[key] = search
	}
	return searches, rows.Err()
}

// RecordBoardSearch records how far the company's board search went at
// searchedAt: the providers that answered without a board, and the board
// found, or nil when none was.
func (s *Store) RecordBoardSearch(ctx context.Context, companyName string, searchedProviders []string, jobBoardID *uuid.UUID,
	searchedAt time.Time) error {
	if searchedProviders == nil {
		searchedProviders = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO board_searches (company_key, company_name, searched_at, searched_providers, job_board_id) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (company_key) DO UPDATE SET company_name = EXCLUDED.company_name, searched_at = EXCLUDED.searched_at,
			searched_providers = EXCLUDED.searched_providers, job_board_id = EXCLUDED.job_board_id`,
		NormalizeCompanyName(companyName), companyName, searchedAt, searchedProviders, jobBoardID)
	return err
}

// tieJobBoardsToCompanies gives each found board without a company the hub's
// company of its name. Its jobs carry the same name, so
// tieJobsToCompanies ties them.
func tieJobBoardsToCompanies(ctx context.Context, tx pgx.Tx) error {
	companyIDs, err := getCompanyIDsByName(ctx, tx)
	if err != nil || len(companyIDs) == 0 {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id, company_name FROM job_boards WHERE company_id IS NULL`)
	if err != nil {
		return err
	}
	matches := map[uuid.UUID]uuid.UUID{}
	for rows.Next() {
		var boardID uuid.UUID
		var companyName string
		if err := rows.Scan(&boardID, &companyName); err != nil {
			return err
		}
		if companyID, found := companyIDs[NormalizeCompanyName(companyName)]; found {
			matches[boardID] = companyID
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for boardID, companyID := range matches {
		if _, err := tx.Exec(ctx, `UPDATE job_boards SET company_id = $2, updated_at = now() WHERE id = $1`, boardID, companyID); err != nil {
			return err
		}
	}
	return nil
}
