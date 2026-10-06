package store

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// knownPeopleShown is how many of the people the owner knows at a company
// its row names, as faces.
const knownPeopleShown = 3

// CompanySummary is one row of the companies list: a company with its watch
// status, job boards and how many people are stored for it.
type CompanySummary struct {
	Company      Company    `json:"company"`
	WatchedSince *time.Time `json:"watched_since,omitempty"`
	JobBoards    []JobBoard `json:"job_boards"`
	PeopleCount  int        `json:"people_count"`
	// ConnectionCount is how many of the owner's connections work here.
	ConnectionCount int `json:"connection_count"`
	// KnownPeople names the closest of those connections, at most three.
	KnownPeople []string `json:"known_people"`
	// ApplicationPhase is the phase of the owner's open application here
	// updated last; absent when none is open.
	ApplicationPhase *string `json:"application_phase,omitempty"`
	// UnseenUpdates counts the company's unseen updates, its jobs' included.
	UnseenUpdates int `json:"unseen_updates"`
}

// ListCompanySummaries returns every stored company, by name, in three
// queries whatever the number of companies.
func (s *Store) ListCompanySummaries(ctx context.Context) ([]CompanySummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+companyColumns+`, watched.added_at, COALESCE(counted.people_count, 0), COALESCE(known.connection_count, 0),
			`+companyUnseenUpdates+`,
			(SELECT pipeline_phases.name FROM applications
			 JOIN pipeline_phases ON pipeline_phases.id = applications.phase_id
			 LEFT JOIN jobs ON jobs.id = applications.job_id
			 WHERE applications.company_id = companies.id AND NOT pipeline_phases.is_closed AND `+cardDismissedAt+` IS NULL
			 ORDER BY applications.updated_at DESC LIMIT 1)
		FROM companies
		LEFT JOIN (SELECT company_id, added_at FROM watch_list_entries WHERE removed_at IS NULL) AS watched
			ON watched.company_id = companies.id
		LEFT JOIN (SELECT company_id, count(*) AS people_count FROM people GROUP BY company_id) AS counted
			ON counted.company_id = companies.id
		LEFT JOIN (SELECT company_id, count(*) AS connection_count FROM connections GROUP BY company_id) AS known
			ON known.company_id = companies.id
		ORDER BY lower(companies.name), companies.name`)
	if err != nil {
		return nil, err
	}
	summaries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CompanySummary, error) {
		summary := CompanySummary{JobBoards: []JobBoard{}, KnownPeople: []string{}}
		company, err := scanCompany(row, &summary.WatchedSince, &summary.PeopleCount, &summary.ConnectionCount, &summary.UnseenUpdates,
			&summary.ApplicationPhase)
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
		if board.CompanyID != nil {
			boardsByCompany[*board.CompanyID] = append(boardsByCompany[*board.CompanyID], board)
		}
	}

	knownPeople, err := s.listKnownPeopleByCompany(ctx)
	if err != nil {
		return nil, err
	}
	for index := range summaries {
		id := summaries[index].Company.ID
		if companyBoards, found := boardsByCompany[id]; found {
			summaries[index].JobBoards = companyBoards
		}
		if names, found := knownPeople[id]; found {
			summaries[index].KnownPeople = names
		}
	}
	return summaries, nil
}

// listKnownPeopleByCompany names the closest connections at each company,
// as ListCompanyConnections ranks them, up to knownPeopleShown.
func (s *Store) listKnownPeopleByCompany(ctx context.Context) (map[uuid.UUID][]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+connectionColumns+` FROM connections WHERE company_id IS NOT NULL
		ORDER BY connected_on NULLS LAST, lower(last_name), last_name`)
	if err != nil {
		return nil, err
	}
	connections, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Connection, error) { return scanConnection(row) })
	if err != nil {
		return nil, err
	}
	byCompany := map[uuid.UUID][]Connection{}
	for _, connection := range connections {
		byCompany[*connection.CompanyID] = append(byCompany[*connection.CompanyID], connection)
	}
	now := time.Now()
	names := map[uuid.UUID][]string{}
	for companyID, atCompany := range byCompany {
		for _, connection := range rankByCloseness(atCompany, now)[:min(len(atCompany), knownPeopleShown)] {
			names[companyID] = append(names[companyID], strings.TrimSpace(connection.FirstName+" "+connection.LastName))
		}
	}
	return names, nil
}
