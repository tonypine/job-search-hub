package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MarketGap is a technology good fits keep asking for that the knowledge
// base never names: how many of the good fits ask for it, which, and the
// plan to close it, learning it or showing it in a portfolio piece.
type MarketGap struct {
	Technology string      `json:"technology"`
	JobCount   int         `json:"job_count"`
	GoodFits   int         `json:"good_fits"`
	JobIDs     []uuid.UUID `json:"job_ids"`
	PlanKind   string      `json:"plan_kind"`
	Plan       string      `json:"plan"`
	ComputedAt time.Time   `json:"computed_at"`
}

// SaveMarketGaps replaces the stored gaps with these.
func (s *Store) SaveMarketGaps(ctx context.Context, gaps []MarketGap) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM market_gaps`); err != nil {
			return err
		}
		for _, gap := range gaps {
			if _, err := tx.Exec(ctx, `
				INSERT INTO market_gaps (technology, job_count, good_fits, job_ids, plan_kind, plan, computed_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				gap.Technology, gap.JobCount, gap.GoodFits, gap.JobIDs, gap.PlanKind, gap.Plan, gap.ComputedAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListMarketGaps returns the stored gaps, the most asked for first.
func (s *Store) ListMarketGaps(ctx context.Context) ([]MarketGap, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT technology, job_count, good_fits, job_ids, plan_kind, plan, computed_at FROM market_gaps ORDER BY job_count DESC, technology`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MarketGap])
}

// GetMarketGapsComputedAt returns when the gaps were last computed; nil
// before the first time.
func (s *Store) GetMarketGapsComputedAt(ctx context.Context) (*time.Time, error) {
	var computedAt *time.Time
	err := s.pool.QueryRow(ctx, `SELECT max(computed_at) FROM market_gaps`).Scan(&computedAt)
	return computedAt, err
}
