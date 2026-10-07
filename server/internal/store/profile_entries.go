package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrProfileEntryNotFound = errors.New("profile entry not found")
	// ErrOwnerConfirmsOnly refuses a confirmation by anyone but the owner.
	ErrOwnerConfirmsOnly = errors.New("only the owner confirms profile entries")
)

// The kinds of profile entry.
const (
	ProfileEntryRole       = "role"
	ProfileEntryCase       = "case"
	ProfileEntrySkill      = "skill"
	ProfileEntryEducation  = "education"
	ProfileEntryProject    = "project"
	ProfileEntryPreference = "preference"
	ProfileEntryFact       = "fact"
)

// Where a profile entry came from.
const (
	ProfileSourceCV        = "cv"
	ProfileSourceLinkedIn  = "linkedin"
	ProfileSourceInterview = "interview"
	ProfileSourceOwner     = "owner"
)

var (
	profileEntryKinds = []string{ProfileEntryRole, ProfileEntryCase, ProfileEntrySkill, ProfileEntryEducation, ProfileEntryProject, ProfileEntryPreference, ProfileEntryFact}
	profileSources    = []string{ProfileSourceCV, ProfileSourceLinkedIn, ProfileSourceInterview, ProfileSourceOwner}
	kindsWithinARole  = []string{ProfileEntryCase, ProfileEntrySkill, ProfileEntryProject}
	profileEntryMonth = regexp.MustCompile(`^[0-9]{4}(-(0[1-9]|1[0-2]))?$`)
)

// ProfileEntry is one piece of the owner's experience: a role held, a case of
// work within one, a skill, and so on. Only a confirmed entry speaks for the
// owner.
type ProfileEntry struct {
	ID   uuid.UUID `json:"id"`
	Kind string    `json:"kind"`
	// RoleID ties a case, skill or project to the role it happened in.
	RoleID       *uuid.UUID `json:"role_id,omitempty"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Organization string     `json:"organization"`
	// StartMonth and EndMonth are YYYY or YYYY-MM; empty is unknown, and an
	// empty EndMonth on a role may mean it's current.
	StartMonth   string     `json:"start_month"`
	EndMonth     string     `json:"end_month"`
	Skills       []string   `json:"skills"`
	Outcome      string     `json:"outcome"`
	Source       string     `json:"source"`
	SourceDetail string     `json:"source_detail"`
	ConfirmedAt  *time.Time `json:"confirmed_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ProfileEntryInput is an entry to add or replace.
type ProfileEntryInput struct {
	Kind         string     `json:"kind"`
	RoleID       *uuid.UUID `json:"role_id,omitempty"`
	Title        string     `json:"title"`
	Body         string     `json:"body,omitempty"`
	Organization string     `json:"organization,omitempty"`
	StartMonth   string     `json:"start_month,omitempty"`
	EndMonth     string     `json:"end_month,omitempty"`
	Skills       []string   `json:"skills,omitempty"`
	Outcome      string     `json:"outcome,omitempty"`
	Source       string     `json:"source"`
	SourceDetail string     `json:"source_detail,omitempty"`
}

// ProfileEntryFilter narrows the list: one kind, or confirmed or unconfirmed
// entries only.
type ProfileEntryFilter struct {
	Kind      string
	Confirmed *bool
}

const profileEntryColumns = `id, kind, role_id, title, body, organization, start_month, end_month, skills, outcome, source, source_detail,
	confirmed_at, updated_at`

func scanProfileEntry(row pgx.Row) (ProfileEntry, error) {
	var entry ProfileEntry
	err := row.Scan(&entry.ID, &entry.Kind, &entry.RoleID, &entry.Title, &entry.Body, &entry.Organization, &entry.StartMonth, &entry.EndMonth,
		&entry.Skills, &entry.Outcome, &entry.Source, &entry.SourceDetail, &entry.ConfirmedAt, &entry.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProfileEntry{}, ErrProfileEntryNotFound
	}
	return entry, err
}

// ListProfileEntries returns the entries the filter keeps, the latest work
// first.
func (s *Store) ListProfileEntries(ctx context.Context, filter ProfileEntryFilter) ([]ProfileEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+profileEntryColumns+` FROM profile_entries
		WHERE ($1 = '' OR kind = $1) AND ($2::boolean IS NULL OR (confirmed_at IS NOT NULL) = $2)
		ORDER BY start_month DESC, created_at, title`, filter.Kind, filter.Confirmed)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ProfileEntry, error) { return scanProfileEntry(row) })
}

// SaveProfileEntry adds an entry, or replaces the one with id. A new entry
// starts unconfirmed. An agent's change unconfirms an entry, since only the
// owner vouches for it; the owner's own change keeps its confirmation.
func (s *Store) SaveProfileEntry(ctx context.Context, actor Actor, id *uuid.UUID, input ProfileEntryInput) (ProfileEntry, error) {
	input, err := cleanProfileEntryInput(input)
	if err != nil {
		return ProfileEntry{}, err
	}
	var saved ProfileEntry
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if input.RoleID != nil {
			var roleKind string
			err := tx.QueryRow(ctx, `SELECT kind FROM profile_entries WHERE id = $1`, *input.RoleID).Scan(&roleKind)
			if errors.Is(err, pgx.ErrNoRows) || roleKind != ProfileEntryRole {
				return fmt.Errorf("role_id must be a role entry's id")
			}
			if err != nil {
				return err
			}
		}
		operation := "create"
		if id == nil {
			saved, err = scanProfileEntry(tx.QueryRow(ctx, `
				INSERT INTO profile_entries (kind, role_id, title, body, organization, start_month, end_month, skills, outcome, source, source_detail)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				RETURNING `+profileEntryColumns,
				input.Kind, input.RoleID, input.Title, input.Body, input.Organization, input.StartMonth, input.EndMonth, input.Skills,
				input.Outcome, input.Source, input.SourceDetail))
		} else {
			operation = "update"
			saved, err = scanProfileEntry(tx.QueryRow(ctx, `
				UPDATE profile_entries SET kind = $2, role_id = $3, title = $4, body = $5, organization = $6, start_month = $7, end_month = $8,
					skills = $9, outcome = $10, source = $11, source_detail = $12,
					confirmed_at = CASE WHEN $13 THEN confirmed_at END, updated_at = now()
				WHERE id = $1 RETURNING `+profileEntryColumns,
				*id, input.Kind, input.RoleID, input.Title, input.Body, input.Organization, input.StartMonth, input.EndMonth, input.Skills,
				input.Outcome, input.Source, input.SourceDetail, actor.Kind == ActorOwner))
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "profile_entry", entityID: saved.ID, operation: operation, after: input})
	})
	return saved, err
}

// CountConfirmedProfileEntries returns how many entries the owner confirmed.
func (s *Store) CountConfirmedProfileEntries(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM profile_entries WHERE confirmed_at IS NOT NULL`).Scan(&count)
	return count, err
}

// ConfirmProfileEntries marks entries as the owner's own account.
func (s *Store) ConfirmProfileEntries(ctx context.Context, actor Actor, ids []uuid.UUID) ([]ProfileEntry, error) {
	if actor.Kind != ActorOwner {
		return nil, ErrOwnerConfirmsOnly
	}
	var confirmed []ProfileEntry
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, id := range ids {
			entry, err := scanProfileEntry(tx.QueryRow(ctx, `
				UPDATE profile_entries SET confirmed_at = coalesce(confirmed_at, now()) WHERE id = $1 RETURNING `+profileEntryColumns, id))
			if err != nil {
				return err
			}
			if err := insertChange(ctx, tx, actor, change{entityType: "profile_entry", entityID: id, operation: "confirm"}); err != nil {
				return err
			}
			confirmed = append(confirmed, entry)
		}
		return nil
	})
	return confirmed, err
}

// DeleteProfileEntry removes an entry. A role's cases stay, no longer tied to
// it.
func (s *Store) DeleteProfileEntry(ctx context.Context, actor Actor, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		deleted, err := scanProfileEntry(tx.QueryRow(ctx, `DELETE FROM profile_entries WHERE id = $1 RETURNING `+profileEntryColumns, id))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "profile_entry", entityID: deleted.ID, operation: "delete",
			before: map[string]string{"kind": deleted.Kind, "title": deleted.Title},
		})
	})
}

func cleanProfileEntryInput(input ProfileEntryInput) (ProfileEntryInput, error) {
	input.Kind, input.Source = strings.TrimSpace(input.Kind), strings.TrimSpace(input.Source)
	input.Title, input.Body, input.Organization = strings.TrimSpace(input.Title), strings.TrimSpace(input.Body), strings.TrimSpace(input.Organization)
	input.StartMonth, input.EndMonth = strings.TrimSpace(input.StartMonth), strings.TrimSpace(input.EndMonth)
	input.Outcome, input.SourceDetail = strings.TrimSpace(input.Outcome), strings.TrimSpace(input.SourceDetail)
	switch {
	case !slices.Contains(profileEntryKinds, input.Kind):
		return input, fmt.Errorf("kind must be one of %s", strings.Join(profileEntryKinds, ", "))
	case !slices.Contains(profileSources, input.Source):
		return input, fmt.Errorf("source must be one of %s", strings.Join(profileSources, ", "))
	case input.Title == "":
		return input, errors.New("an entry needs a title")
	case input.RoleID != nil && !slices.Contains(kindsWithinARole, input.Kind):
		return input, errors.New("only a case, skill or project belongs to a role")
	}
	for _, month := range []string{input.StartMonth, input.EndMonth} {
		if month != "" && !profileEntryMonth.MatchString(month) {
			return input, fmt.Errorf("months are YYYY or YYYY-MM, not %q", month)
		}
	}
	skills := []string{}
	for _, skill := range input.Skills {
		if skill = strings.TrimSpace(skill); skill != "" && !slices.ContainsFunc(skills, func(kept string) bool { return strings.EqualFold(kept, skill) }) {
			skills = append(skills, skill)
		}
	}
	input.Skills = skills
	return input, nil
}
