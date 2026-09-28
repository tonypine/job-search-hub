package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// CompanyDossier is everything the hub knows about one company.
type CompanyDossier struct {
	Company      Company    `json:"company"`
	WatchedSince *time.Time `json:"watched_since,omitempty" jsonschema:"when the company was put on the watch list; absent when it is not on it"`
	JobBoards    []JobBoard `json:"job_boards"`
	People       []Person   `json:"people"`
	// Connections are the owner's LinkedIn connections who work here: warm
	// paths, to try before anyone the owner doesn't know.
	Connections []Connection `json:"connections" jsonschema:"the owner's LinkedIn connections who work here: warm paths to try before anyone the owner doesn't know"`
}

func (s *Store) GetCompanyDossier(ctx context.Context, companyID uuid.UUID) (CompanyDossier, error) {
	company, err := s.GetCompany(ctx, companyID)
	if err != nil {
		return CompanyDossier{}, err
	}
	watchedSince, err := s.GetWatchedSince(ctx, companyID)
	if err != nil {
		return CompanyDossier{}, err
	}
	jobBoards, err := s.ListJobBoards(ctx, companyID)
	if err != nil {
		return CompanyDossier{}, err
	}
	people, err := s.ListPeople(ctx, companyID)
	if err != nil {
		return CompanyDossier{}, err
	}
	connections, err := s.ListCompanyConnections(ctx, companyID)
	if err != nil {
		return CompanyDossier{}, err
	}
	return CompanyDossier{Company: company, WatchedSince: watchedSince, JobBoards: jobBoards, People: people, Connections: connections}, nil
}
