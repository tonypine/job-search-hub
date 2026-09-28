package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CompanySummary is one row of the companies list: a company with its watch
// status, job boards and how many people are stored for it.
type CompanySummary struct {
	Company      Company    `json:"company"`
	WatchedSince *time.Time `json:"watched_since,omitempty"`
	JobBoards    []JobBoard `json:"job_boards"`
	PeopleCount  int        `json:"people_count"`
}

// ListCompanySummaries returns every stored company, by name, in two queries
// whatever the number of companies.
func (s *Store) ListCompanySummaries(ctx context.Context) ([]CompanySummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+companyColumns+`, watched.added_at, COALESCE(counted.people_count, 0)
		FROM companies
		LEFT JOIN (SELECT company_id, added_at FROM watch_list_entries WHERE removed_at IS NULL) AS watched
			ON watched.company_id = companies.id
		LEFT JOIN (SELECT company_id, count(*) AS people_count FROM people GROUP BY company_id) AS counted
			ON counted.company_id = companies.id
		ORDER BY companies.name`)
	if err != nil {
		return nil, err
	}
	summaries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CompanySummary, error) {
		summary := CompanySummary{JobBoards: []JobBoard{}}
		company, err := scanCompany(row, &summary.WatchedSince, &summary.PeopleCount)
		summary.Company = company
		return summary, err
	})
	if err != nil {
		return nil, err
	}

	boardRows, err := s.pool.Query(ctx, `SELECT `+jobBoardColumns+` FROM job_boards ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	boards, err := pgx.CollectRows(boardRows, func(row pgx.CollectableRow) (JobBoard, error) { return scanJobBoard(row) })
	if err != nil {
		return nil, err
	}
	boardsByCompany := map[uuid.UUID][]JobBoard{}
	for _, board := range boards {
		boardsByCompany[board.CompanyID] = append(boardsByCompany[board.CompanyID], board)
	}
	for index := range summaries {
		if companyBoards, found := boardsByCompany[summaries[index].Company.ID]; found {
			summaries[index].JobBoards = companyBoards
		}
	}
	return summaries, nil
}
