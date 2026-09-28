package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrCompanyNotFound = errors.New("company not found")

const maximumFoundCompanies = 20

type Company struct {
	ID                  uuid.UUID `json:"id"`
	Name                string    `json:"name"`
	Domain              string    `json:"domain"`
	WebsiteURL          string    `json:"website_url,omitempty"`
	CareersURL          string    `json:"careers_url,omitempty"`
	HeadquartersCountry string    `json:"headquarters_country,omitempty"`
	EmployeeCountRange  string    `json:"employee_count_range,omitempty"`
	Summary             string    `json:"summary,omitempty"`
	FoundVia            string    `json:"found_via,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

const companyColumns = `id, name, domain, website_url, careers_url, headquarters_country, employee_count_range, summary, found_via, created_at, updated_at`

// scanCompany reads the companyColumns, then any extra columns the query
// selects after them into extra.
func scanCompany(row pgx.Row, extra ...any) (Company, error) {
	var company Company
	destinations := append([]any{&company.ID, &company.Name, &company.Domain, &company.WebsiteURL, &company.CareersURL,
		&company.HeadquartersCountry, &company.EmployeeCountRange, &company.Summary, &company.FoundVia, &company.CreatedAt, &company.UpdatedAt}, extra...)
	err := row.Scan(destinations...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Company{}, ErrCompanyNotFound
	}
	return company, err
}

type NewCompany struct {
	Name       string
	Domain     string
	WebsiteURL string
	FoundVia   string
	SourceURL  string
}

// CreateCompany stores a company, or returns the one already stored under the
// same normalized domain. created reports which, and only a created company
// records a change.
func (s *Store) CreateCompany(ctx context.Context, actor Actor, input NewCompany) (Company, bool, error) {
	domain, err := NormalizeDomain(input.Domain)
	if err != nil {
		return Company{}, false, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Company{}, false, errors.New("a company needs a name")
	}

	var company Company
	created := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		inserted, insertErr := scanCompany(tx.QueryRow(ctx, `
			INSERT INTO companies (name, domain, website_url, found_via) VALUES ($1, $2, $3, $4)
			ON CONFLICT (domain) DO NOTHING
			RETURNING `+companyColumns, name, domain, input.WebsiteURL, input.FoundVia))
		if errors.Is(insertErr, ErrCompanyNotFound) {
			existing, selectErr := scanCompany(tx.QueryRow(ctx, `SELECT `+companyColumns+` FROM companies WHERE domain = $1`, domain))
			company = existing
			return selectErr
		}
		if insertErr != nil {
			return insertErr
		}
		company = inserted
		created = true
		if err := matchCompanyNames(ctx, tx); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "company", entityID: company.ID, operation: "create", after: company, sourceURL: input.SourceURL,
		})
	})
	if err != nil {
		return Company{}, false, err
	}
	return company, created, nil
}

func (s *Store) GetCompany(ctx context.Context, id uuid.UUID) (Company, error) {
	return scanCompany(s.pool.QueryRow(ctx, `SELECT `+companyColumns+` FROM companies WHERE id = $1`, id))
}

func (s *Store) GetCompanyByDomain(ctx context.Context, rawDomain string) (Company, error) {
	domain, err := NormalizeDomain(rawDomain)
	if err != nil {
		return Company{}, err
	}
	return scanCompany(s.pool.QueryRow(ctx, `SELECT `+companyColumns+` FROM companies WHERE domain = $1`, domain))
}

// FindCompanies matches the query against names, case-insensitively, and
// against domains; a URL in the query is matched by its domain.
func (s *Store) FindCompanies(ctx context.Context, query string) ([]Company, error) {
	nameQuery := strings.ToLower(strings.TrimSpace(query))
	domainQuery := nameQuery
	if domain, err := NormalizeDomain(query); err == nil {
		domainQuery = domain
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+companyColumns+` FROM companies
		WHERE strpos(lower(name), $1) > 0 OR strpos(domain, $2) > 0
		ORDER BY name LIMIT $3`, nameQuery, domainQuery, maximumFoundCompanies)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Company, error) { return scanCompany(row) })
}

// CompanyUpdate sets each non-nil field. The domain is the company's identity
// and is not updatable.
type CompanyUpdate struct {
	Name                *string
	WebsiteURL          *string
	CareersURL          *string
	HeadquartersCountry *string
	EmployeeCountRange  *string
	Summary             *string
	FoundVia            *string
	SourceURL           string
}

// UpdateCompany applies the update and records the fields that actually
// changed, with their values before and after. An update that changes nothing
// records nothing.
func (s *Store) UpdateCompany(ctx context.Context, actor Actor, id uuid.UUID, update CompanyUpdate) (Company, error) {
	var company Company
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanCompany(tx.QueryRow(ctx, `SELECT `+companyColumns+` FROM companies WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}

		fields := []struct {
			column  string
			current string
			next    *string
		}{
			{"name", current.Name, update.Name},
			{"website_url", current.WebsiteURL, update.WebsiteURL},
			{"careers_url", current.CareersURL, update.CareersURL},
			{"headquarters_country", current.HeadquartersCountry, update.HeadquartersCountry},
			{"employee_count_range", current.EmployeeCountRange, update.EmployeeCountRange},
			{"summary", current.Summary, update.Summary},
			{"found_via", current.FoundVia, update.FoundVia},
		}
		before := map[string]string{}
		after := map[string]string{}
		for _, field := range fields {
			if field.next != nil && *field.next != field.current {
				before[field.column] = field.current
				after[field.column] = *field.next
			}
		}
		if len(after) == 0 {
			company = current
			return nil
		}

		company, err = scanCompany(tx.QueryRow(ctx, `
			UPDATE companies SET
				name = COALESCE($2, name),
				website_url = COALESCE($3, website_url),
				careers_url = COALESCE($4, careers_url),
				headquarters_country = COALESCE($5, headquarters_country),
				employee_count_range = COALESCE($6, employee_count_range),
				summary = COALESCE($7, summary),
				found_via = COALESCE($8, found_via),
				updated_at = now()
			WHERE id = $1
			RETURNING `+companyColumns,
			id, update.Name, update.WebsiteURL, update.CareersURL, update.HeadquartersCountry, update.EmployeeCountRange, update.Summary, update.FoundVia))
		if err != nil {
			return err
		}
		if _, renamed := after["name"]; renamed {
			if err := matchCompanyNames(ctx, tx); err != nil {
				return err
			}
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "company", entityID: id, operation: "update", before: before, after: after, sourceURL: update.SourceURL,
		})
	})
	return company, err
}

// NormalizeDomain reduces a URL or host to the bare, lowercased domain a
// company is stored under: no scheme, port, path, trailing dot or leading "www.".
func NormalizeDomain(raw string) (string, error) {
	candidate := strings.ToLower(strings.TrimSpace(raw))
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("%q is not a domain: %w", raw, err)
	}
	host := strings.TrimPrefix(strings.TrimSuffix(parsed.Hostname(), "."), "www.")
	if !strings.Contains(host, ".") || strings.ContainsAny(host, " _") {
		return "", fmt.Errorf("%q is not a domain", raw)
	}
	return host, nil
}

// matchCompanyNames ties the connections and the jobs that name a company to
// the hub company of that name, as when a company is added or renamed.
func matchCompanyNames(ctx context.Context, tx pgx.Tx) error {
	if _, err := matchConnectionsToCompanies(ctx, tx); err != nil {
		return err
	}
	_, err := tieJobsToCompanies(ctx, tx)
	return err
}
