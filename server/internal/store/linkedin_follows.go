package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type NewCompanyFollow struct {
	Organization string
	FollowedAt   *time.Time
}

// FollowsImport counts the followed companies an import stored.
type FollowsImport struct {
	Stored int `json:"stored"`
}

// ImportCompanyFollows stores each followed company once, by name.
func (s *Store) ImportCompanyFollows(ctx context.Context, actor Actor, follows []NewCompanyFollow) (FollowsImport, error) {
	var result FollowsImport
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, follow := range follows {
			if _, err := tx.Exec(ctx, `
				INSERT INTO linkedin_company_follows (organization, followed_at) VALUES ($1, $2)
				ON CONFLICT (organization) DO UPDATE SET followed_at = EXCLUDED.followed_at`, follow.Organization, follow.FollowedAt); err != nil {
				return err
			}
			result.Stored++
		}
		return insertChange(ctx, tx, actor, change{entityType: "linkedin_company_follows", entityID: uuid.New(), operation: "import", after: result})
	})
	return result, err
}

// CompanyFollow is a company the owner follows, and how many of the owner's
// connections work there.
type CompanyFollow struct {
	Organization    string     `json:"organization"`
	FollowedAt      *time.Time `json:"followed_at,omitempty"`
	ConnectionCount int        `json:"connection_count"`
}

// ListCompanyFollowsNotInHub returns the followed companies the hub doesn't
// hold yet, compared by name, with the connections at each.
func (s *Store) ListCompanyFollowsNotInHub(ctx context.Context) ([]CompanyFollow, error) {
	var follows []CompanyFollow
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		companyIDs, err := getCompanyIDsByName(ctx, tx)
		if err != nil {
			return err
		}
		connectionCounts := map[string]int{}
		rows, err := tx.Query(ctx, `SELECT company_name FROM connections WHERE company_name <> ''`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var companyName string
			if err := rows.Scan(&companyName); err != nil {
				return err
			}
			connectionCounts[NormalizeCompanyName(companyName)]++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `SELECT organization, followed_at FROM linkedin_company_follows ORDER BY followed_at DESC NULLS LAST`)
		if err != nil {
			return err
		}
		all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CompanyFollow, error) {
			var follow CompanyFollow
			err := row.Scan(&follow.Organization, &follow.FollowedAt)
			return follow, err
		})
		if err != nil {
			return err
		}
		for _, follow := range all {
			key := NormalizeCompanyName(follow.Organization)
			if _, inHub := companyIDs[key]; inHub {
				continue
			}
			follow.ConnectionCount = connectionCounts[key]
			follows = append(follows, follow)
		}
		return nil
	})
	return follows, err
}
