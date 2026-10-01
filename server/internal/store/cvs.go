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
	// HasPDF says the owner printed it to a PDF the hub keeps.
	HasPDF    bool      `json:"has_pdf"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const cvColumns = `id, kind, job_id, content, citations, pdf IS NOT NULL, created_at, updated_at`

// prefixedCVColumns are the cvColumns qualified for queries that join jobs.
const prefixedCVColumns = `cvs.id, cvs.kind, cvs.job_id, cvs.content, cvs.citations, cvs.pdf IS NOT NULL, cvs.created_at, cvs.updated_at`

func scanCV(row pgx.Row) (CV, error) {
	var cv CV
	var content, citations json.RawMessage
	err := row.Scan(&cv.ID, &cv.Kind, &cv.JobID, &content, &citations, &cv.HasPDF, &cv.CreatedAt, &cv.UpdatedAt)
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

// SaveTailoredCV keeps content as the job's tailored CV, replacing the one
// before, with each bullet's source in citations.
func (s *Store) SaveTailoredCV(ctx context.Context, actor Actor, jobID uuid.UUID, content resume.Resume, citations map[string]string) (CV, error) {
	encoded, err := json.Marshal(content)
	if err != nil {
		return CV{}, err
	}
	sources, err := json.Marshal(citations)
	if err != nil {
		return CV{}, err
	}
	var saved CV
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		saved, err = scanCV(tx.QueryRow(ctx, `
			INSERT INTO cvs (kind, job_id, content, citations) VALUES ('tailored', $1, $2, $3)
			ON CONFLICT (job_id) WHERE kind = 'tailored' DO UPDATE SET content = EXCLUDED.content, citations = EXCLUDED.citations, pdf = NULL, updated_at = now()
			RETURNING `+cvColumns, jobID, encoded, sources))
		if isForeignKeyViolation(err) {
			return ErrJobNotFound
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "cv", entityID: saved.ID, operation: "draft for job", after: map[string]string{"job_id": jobID.String()}})
	})
	return saved, err
}

// GetJobCV returns the job's tailored CV.
func (s *Store) GetJobCV(ctx context.Context, jobID uuid.UUID) (CV, error) {
	return scanCV(s.pool.QueryRow(ctx, `SELECT `+cvColumns+` FROM cvs WHERE kind = 'tailored' AND job_id = $1`, jobID))
}

// ListJobsAwaitingCV returns the open jobs the owner decided to pursue that
// have no tailored CV yet, the most recently pursued first.
func (s *Store) ListJobsAwaitingCV(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT jobs.id FROM jobs JOIN job_decisions ON job_decisions.job_id = jobs.id AND job_decisions.decision = 'pursue'
		WHERE jobs.closed_at IS NULL AND NOT EXISTS (SELECT 1 FROM cvs WHERE cvs.kind = 'tailored' AND cvs.job_id = jobs.id)
		ORDER BY job_decisions.decided_at DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// MaximumCVPDFSize bounds a CV's PDF.
const MaximumCVPDFSize = 5 << 20

// SaveCVPDF keeps the PDF the owner printed of the CV. A CV saved again
// drops its PDF, which no longer matches it.
func (s *Store) SaveCVPDF(ctx context.Context, actor Actor, id uuid.UUID, pdf []byte) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE cvs SET pdf = $2 WHERE id = $1`, id, pdf)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrCVNotFound
		}
		return insertChange(ctx, tx, actor, change{entityType: "cv", entityID: id, operation: "print"})
	})
}

// GetCVPDF returns the CV's PDF.
func (s *Store) GetCVPDF(ctx context.Context, id uuid.UUID) ([]byte, error) {
	var pdf []byte
	err := s.pool.QueryRow(ctx, `SELECT pdf FROM cvs WHERE id = $1 AND pdf IS NOT NULL`, id).Scan(&pdf)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCVNotFound
	}
	return pdf, err
}
