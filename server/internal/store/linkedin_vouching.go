package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Whether a vouch was received by the owner or given by them.
const (
	VouchedReceived = "received"
	VouchedGiven    = "given"
)

type NewLinkedInEndorsement struct {
	Direction  string
	Skill      string
	FirstName  string
	LastName   string
	ProfileURL string
	EndorsedAt *time.Time
	Status     string
}

type NewLinkedInRecommendation struct {
	Direction string
	FirstName string
	LastName  string
	Company   string
	JobTitle  string
	Text      string
	WrittenAt *time.Time
	Status    string
}

// VouchingImport counts what an import of endorsements or recommendations
// stored, and how many connections now vouch for, or are vouched for by, the
// owner.
type VouchingImport struct {
	Stored                 int `json:"stored"`
	ConnectionsWithVouches int `json:"connections_with_vouches"`
}

// ImportLinkedInEndorsements stores each endorsement once, by its direction,
// person and skill, then refreshes what joins each connection and the owner.
func (s *Store) ImportLinkedInEndorsements(ctx context.Context, actor Actor, endorsements []NewLinkedInEndorsement) (VouchingImport, error) {
	var result VouchingImport
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, endorsement := range endorsements {
			if _, err := tx.Exec(ctx, `
				INSERT INTO linkedin_endorsements (direction, skill, first_name, last_name, profile_url, endorsed_at, status)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (direction, profile_url, skill) DO UPDATE SET
					first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name, endorsed_at = EXCLUDED.endorsed_at, status = EXCLUDED.status`,
				endorsement.Direction, endorsement.Skill, endorsement.FirstName, endorsement.LastName, endorsement.ProfileURL,
				endorsement.EndorsedAt, endorsement.Status); err != nil {
				return err
			}
			result.Stored++
		}
		var err error
		if result.ConnectionsWithVouches, err = refreshConnectionVouches(ctx, tx); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "linkedin_endorsements", entityID: uuid.New(), operation: "import", after: result})
	})
	return result, err
}

// ImportLinkedInRecommendations stores each recommendation once, by its
// direction, person and date, then refreshes each connection's vouches.
func (s *Store) ImportLinkedInRecommendations(ctx context.Context, actor Actor, recommendations []NewLinkedInRecommendation) (VouchingImport, error) {
	var result VouchingImport
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, recommendation := range recommendations {
			if _, err := tx.Exec(ctx, `
				INSERT INTO linkedin_recommendations (direction, first_name, last_name, company, job_title, text, written_at, status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (direction, first_name, last_name, written_at) DO UPDATE SET
					company = EXCLUDED.company, job_title = EXCLUDED.job_title, text = EXCLUDED.text, status = EXCLUDED.status`,
				recommendation.Direction, recommendation.FirstName, recommendation.LastName, recommendation.Company,
				recommendation.JobTitle, recommendation.Text, recommendation.WrittenAt, recommendation.Status); err != nil {
				return err
			}
			result.Stored++
		}
		var err error
		if result.ConnectionsWithVouches, err = refreshConnectionVouches(ctx, tx); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "linkedin_recommendations", entityID: uuid.New(), operation: "import", after: result})
	})
	return result, err
}

// refreshConnectionVouches recounts, for every connection, the skills they
// endorsed the owner for, whether the owner endorsed them, and the
// recommendations either way: endorsements by profile URL, recommendations
// by name, since the export gives recommendations no URL. It returns how many
// connections have any vouch.
func refreshConnectionVouches(ctx context.Context, tx pgx.Tx) (int, error) {
	var withVouches int
	err := tx.QueryRow(ctx, `
		WITH updated AS (
			UPDATE connections SET
				endorsed_owner_for = COALESCE((
					SELECT array_agg(DISTINCT skill ORDER BY skill) FROM linkedin_endorsements
					WHERE direction = 'received' AND status <> 'rejected'
						AND lower(rtrim(profile_url, '/')) = lower(rtrim(connections.profile_url, '/'))), '{}'),
				owner_endorsed = EXISTS (
					SELECT 1 FROM linkedin_endorsements
					WHERE direction = 'given' AND lower(rtrim(profile_url, '/')) = lower(rtrim(connections.profile_url, '/'))),
				recommended_owner = EXISTS (
					SELECT 1 FROM linkedin_recommendations
					WHERE direction = 'received' AND lower(first_name) = lower(connections.first_name) AND lower(last_name) = lower(connections.last_name)),
				owner_recommended = EXISTS (
					SELECT 1 FROM linkedin_recommendations
					WHERE direction = 'given' AND lower(first_name) = lower(connections.first_name) AND lower(last_name) = lower(connections.last_name))
			RETURNING endorsed_owner_for, owner_endorsed, recommended_owner, owner_recommended
		)
		SELECT count(*) FROM updated
		WHERE cardinality(endorsed_owner_for) > 0 OR owner_endorsed OR recommended_owner OR owner_recommended`).Scan(&withVouches)
	return withVouches, err
}

// Recommendation is one recommendation someone wrote about the owner.
type Recommendation struct {
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Company   string     `json:"company,omitempty"`
	JobTitle  string     `json:"job_title,omitempty"`
	Text      string     `json:"text"`
	WrittenAt *time.Time `json:"written_at,omitempty"`
}

// ListRecommendationsReceived returns the visible recommendations written
// about the owner, newest first.
func (s *Store) ListRecommendationsReceived(ctx context.Context) ([]Recommendation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT first_name, last_name, company, job_title, text, written_at FROM linkedin_recommendations
		WHERE direction = 'received' AND status IN ('', 'visible') ORDER BY written_at DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Recommendation, error) {
		var recommendation Recommendation
		err := row.Scan(&recommendation.FirstName, &recommendation.LastName, &recommendation.Company, &recommendation.JobTitle,
			&recommendation.Text, &recommendation.WrittenAt)
		return recommendation, err
	})
}
