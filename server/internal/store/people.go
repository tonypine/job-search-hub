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

// PersonRelevances are why a person is worth contacting.
var PersonRelevances = []string{"hiring_manager", "engineering_lead", "recruiter", "founder", "other"}

type Person struct {
	ID         uuid.UUID `json:"id"`
	CompanyID  uuid.UUID `json:"company_id"`
	Name       string    `json:"name"`
	RoleTitle  string    `json:"role_title,omitempty"`
	Relevance  string    `json:"relevance"`
	ProfileURL string    `json:"profile_url,omitempty"`
	SourceURL  string    `json:"source_url"`
	Notes      string    `json:"notes,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

const personColumns = `id, company_id, name, role_title, relevance, profile_url, source_url, notes, created_at`

func scanPerson(row pgx.Row) (Person, error) {
	var person Person
	err := row.Scan(&person.ID, &person.CompanyID, &person.Name, &person.RoleTitle, &person.Relevance,
		&person.ProfileURL, &person.SourceURL, &person.Notes, &person.CreatedAt)
	return person, err
}

type PersonInput struct {
	CompanyID  uuid.UUID
	Name       string
	RoleTitle  string
	Relevance  string
	ProfileURL string
	SourceURL  string
	Notes      string
}

// AddPerson stores a person at a company, or returns the one already stored
// there under the same name. created reports which. A person without a source
// is refused: every person must be traceable to the page that named them.
func (s *Store) AddPerson(ctx context.Context, actor Actor, input PersonInput) (Person, bool, error) {
	name := strings.TrimSpace(input.Name)
	sourceURL := strings.TrimSpace(input.SourceURL)
	switch {
	case name == "":
		return Person{}, false, errors.New("a person needs a name")
	case !strings.HasPrefix(sourceURL, "https://") && !strings.HasPrefix(sourceURL, "http://"):
		return Person{}, false, errors.New("a person needs a source_url: the http(s) page that names them at this company")
	case !slices.Contains(PersonRelevances, input.Relevance):
		return Person{}, false, fmt.Errorf("relevance must be one of %s", strings.Join(PersonRelevances, ", "))
	}

	var person Person
	created := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		inserted, err := scanPerson(tx.QueryRow(ctx, `
			INSERT INTO people (company_id, name, role_title, relevance, profile_url, source_url, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (company_id, lower(name)) DO NOTHING
			RETURNING `+personColumns,
			input.CompanyID, name, input.RoleTitle, input.Relevance, input.ProfileURL, sourceURL, input.Notes))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			person, err = scanPerson(tx.QueryRow(ctx, `
				SELECT `+personColumns+` FROM people WHERE company_id = $1 AND lower(name) = lower($2)`, input.CompanyID, name))
			return err
		case isForeignKeyViolation(err):
			return ErrCompanyNotFound
		case err != nil:
			return err
		}
		person = inserted
		created = true
		return insertChange(ctx, tx, actor, change{
			entityType: "person", entityID: person.ID, operation: "create", after: person, sourceURL: sourceURL,
		})
	})
	if err != nil {
		return Person{}, false, err
	}
	return person, created, nil
}

func (s *Store) ListPeople(ctx context.Context, companyID uuid.UUID) ([]Person, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+personColumns+` FROM people WHERE company_id = $1 ORDER BY created_at`, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Person, error) { return scanPerson(row) })
}
