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

// contactRelevanceOrder ranks a company's stored contacts as a job's People
// card does: the closest to the hire first, any other relevance after.
const contactRelevanceOrder = `CASE relevance WHEN 'hiring_manager' THEN 0 WHEN 'recruiter' THEN 1
	WHEN 'engineering_lead' THEN 2 WHEN 'founder' THEN 3 WHEN 'engineer' THEN 4 ELSE 5 END`

// CompanySummary is one row of the companies list: a company with its watch
// status, job boards and how many people are stored for it.
type CompanySummary struct {
	Company      Company    `json:"company"`
	WatchedSince *time.Time `json:"watched_since,omitempty"`
	JobBoards    []JobBoard `json:"job_boards"`
	PeopleCount  int        `json:"people_count"`
	// ConnectionCount is how many of the owner's connections work here.
	ConnectionCount int `json:"connection_count"`
	// KnownPeopleCount is how many people can get the owner in here, as a
	// job's People card counts them: the connections who work here, the
	// people who can introduce the owner, and the stored contacts.
	KnownPeopleCount int `json:"known_people_count"`
	// KnownPeople names the first of those people, at most three:
	// connections, the closest first, then introducers, then the contacts
	// closest to the hire.
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
			summaries[index].KnownPeopleCount = len(names)
			summaries[index].KnownPeople = names[:min(len(names), knownPeopleShown)]
		}
	}
	return summaries, nil
}

// listKnownPeopleByCompany names everyone who can get the owner in at each
// company, in the order a job's People card lists them: the connections who
// work there, as ListCompanyConnections ranks them, the people who can
// introduce the owner, by name, then the stored contacts, the closest to the
// hire first.
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
		for _, connection := range rankByCloseness(atCompany, now) {
			names[companyID] = append(names[companyID], strings.TrimSpace(connection.FirstName+" "+connection.LastName))
		}
	}

	rows, err = s.pool.Query(ctx, `
		SELECT company_id, name FROM (
			SELECT links.company_id, contacts.name, 0 AS kind, 0 AS relevance_rank
			FROM network_contact_companies AS links JOIN network_contacts AS contacts ON contacts.id = links.contact_id
			UNION ALL
			SELECT company_id, name, 1, `+contactRelevanceOrder+` FROM people
		) AS reachable
		ORDER BY kind, relevance_rank, lower(name), name`)
	if err != nil {
		return nil, err
	}
	type reachablePerson struct {
		companyID uuid.UUID
		name      string
	}
	others, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (reachablePerson, error) {
		var person reachablePerson
		err := row.Scan(&person.companyID, &person.name)
		return person, err
	})
	if err != nil {
		return nil, err
	}
	for _, person := range others {
		names[person.companyID] = append(names[person.companyID], strings.TrimSpace(person.name))
	}
	return names, nil
}
