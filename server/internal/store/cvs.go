package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tonypine/job-search-hub/server/internal/resume"
)

var ErrCVNotFound = errors.New("CV not found")

const (
	CVKindBase     = "base"
	CVKindTailored = "tailored"
)

// CV is the owner's base CV, or a draft tailored for a job. Citations map a
// tailored draft's bullet ("w0h1") to its source: a base bullet ("base:w2h0")
// or a confirmed knowledge-base entry ("entry:<id>").
type CV struct {
	ID        uuid.UUID         `json:"id"`
	Kind      string            `json:"kind"`
	JobID     *uuid.UUID        `json:"job_id,omitempty"`
	Content   resume.Resume     `json:"content"`
	Citations map[string]string `json:"citations"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

const cvColumns = `id, kind, job_id, content, citations, created_at, updated_at`

func scanCV(row pgx.Row) (CV, error) {
	var cv CV
	var content, citations json.RawMessage
	err := row.Scan(&cv.ID, &cv.Kind, &cv.JobID, &content, &citations, &cv.CreatedAt, &cv.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CV{}, ErrCVNotFound
	}
	if err != nil {
		return CV{}, err
	}
	if err := json.Unmarshal(content, &cv.Content); err != nil {
		return CV{}, err
	}
	return cv, json.Unmarshal(citations, &cv.Citations)
}

// GetBaseCV returns the owner's base CV.
func (s *Store) GetBaseCV(ctx context.Context) (CV, error) {
	return scanCV(s.pool.QueryRow(ctx, `SELECT `+cvColumns+` FROM cvs WHERE kind = 'base'`))
}

// GetCV returns a CV by its id.
func (s *Store) GetCV(ctx context.Context, id uuid.UUID) (CV, error) {
	return scanCV(s.pool.QueryRow(ctx, `SELECT `+cvColumns+` FROM cvs WHERE id = $1`, id))
}

// SaveBaseCV replaces the owner's base CV, recording the change.
func (s *Store) SaveBaseCV(ctx context.Context, actor Actor, content resume.Resume) (CV, error) {
	encoded, err := json.Marshal(content)
	if err != nil {
		return CV{}, err
	}
	var saved CV
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		saved, err = scanCV(tx.QueryRow(ctx, `
			INSERT INTO cvs (kind, content) VALUES ('base', $1)
			ON CONFLICT (kind) WHERE kind = 'base' DO UPDATE SET content = EXCLUDED.content, updated_at = now()
			RETURNING `+cvColumns, encoded))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "cv", entityID: saved.ID, operation: "save base"})
	})
	return saved, err
}
