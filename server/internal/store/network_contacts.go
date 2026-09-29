package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrWarmPathNotFound = errors.New("warm path not found")

// WarmPath is someone the owner knows who can open doors at a company
// without working there, and how.
type WarmPath struct {
	ContactID        uuid.UUID `json:"contact_id"`
	Name             string    `json:"name"`
	HowKnown         string    `json:"how_known,omitempty" jsonschema:"how the owner knows them, e.g. a former colleague"`
	PreferredChannel string    `json:"preferred_channel,omitempty" jsonschema:"where they prefer to be reached, e.g. LinkedIn"`
	Note             string    `json:"note,omitempty" jsonschema:"how they can help at this company, e.g. interviewed there"`
}

// NewWarmPath links someone the owner knows to a company.
type NewWarmPath struct {
	Name             string
	HowKnown         string
	PreferredChannel string
	Note             string
}

// AddWarmPath links the contact of that name to the company, adding the
// contact when the hub doesn't know them. How the owner knows them and their
// channel are updated when given; the note is the company's.
func (s *Store) AddWarmPath(ctx context.Context, actor Actor, companyID uuid.UUID, input NewWarmPath) (WarmPath, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return WarmPath{}, errors.New("a warm path needs the person's name")
	}
	var path WarmPath
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO network_contacts (name, how_known, preferred_channel) VALUES ($1, $2, $3)
			ON CONFLICT (lower(btrim(name))) DO UPDATE SET
				how_known = COALESCE(NULLIF(EXCLUDED.how_known, ''), network_contacts.how_known),
				preferred_channel = COALESCE(NULLIF(EXCLUDED.preferred_channel, ''), network_contacts.preferred_channel),
				updated_at = now()
			RETURNING id, name, how_known, preferred_channel`,
			name, strings.TrimSpace(input.HowKnown), strings.TrimSpace(input.PreferredChannel)).
			Scan(&path.ContactID, &path.Name, &path.HowKnown, &path.PreferredChannel); err != nil {
			return err
		}
		path.Note = strings.TrimSpace(input.Note)
		_, err := tx.Exec(ctx, `
			INSERT INTO network_contact_companies (contact_id, company_id, note) VALUES ($1, $2, $3)
			ON CONFLICT (contact_id, company_id) DO UPDATE SET note = EXCLUDED.note`, path.ContactID, companyID, path.Note)
		if isForeignKeyViolation(err) {
			return ErrCompanyNotFound
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "warm_path", entityID: path.ContactID, operation: "link",
			after: map[string]string{"company_id": companyID.String(), "name": path.Name, "note": path.Note},
		})
	})
	return path, err
}

// RemoveWarmPath unlinks a contact from a company, and forgets the contact
// once no company is linked to them.
func (s *Store) RemoveWarmPath(ctx context.Context, actor Actor, companyID, contactID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		removed, err := tx.Exec(ctx, `DELETE FROM network_contact_companies WHERE contact_id = $1 AND company_id = $2`, contactID, companyID)
		if err != nil {
			return err
		}
		if removed.RowsAffected() == 0 {
			return ErrWarmPathNotFound
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM network_contacts WHERE id = $1
			  AND NOT EXISTS (SELECT 1 FROM network_contact_companies WHERE contact_id = $1)`, contactID); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "warm_path", entityID: contactID, operation: "unlink", before: map[string]string{"company_id": companyID.String()},
		})
	})
}

// ListCompanyWarmPaths returns the people the owner knows who can open doors
// at the company, by name.
func (s *Store) ListCompanyWarmPaths(ctx context.Context, companyID uuid.UUID) ([]WarmPath, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT contacts.id, contacts.name, contacts.how_known, contacts.preferred_channel, links.note
		FROM network_contact_companies AS links JOIN network_contacts AS contacts ON contacts.id = links.contact_id
		WHERE links.company_id = $1
		ORDER BY lower(contacts.name)`, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (WarmPath, error) {
		var path WarmPath
		err := row.Scan(&path.ContactID, &path.Name, &path.HowKnown, &path.PreferredChannel, &path.Note)
		return path, err
	})
}
