package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrCVScreenNotFound = errors.New("cv screen not found")

// CVScreen is a recruiter's reading of a job's tailored CV, as it was when
// screened: the likely reasons to reject it, in the shape the
// recruiter_screen prompt's schema gives.
type CVScreen struct {
	JobID       uuid.UUID       `json:"job_id"`
	CVID        uuid.UUID       `json:"cv_id"`
	CVUpdatedAt time.Time       `json:"cv_updated_at"`
	PromptID    uuid.UUID       `json:"prompt_id"`
	Model       string          `json:"model"`
	Screen      json.RawMessage `json:"screen"`
	CreatedAt   time.Time       `json:"created_at"`
}

// SaveCVScreen keeps the job's screen, replacing an earlier one.
func (s *Store) SaveCVScreen(ctx context.Context, screen CVScreen) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO cv_screens (job_id, cv_id, cv_updated_at, prompt_id, model, screen) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (job_id) DO UPDATE SET cv_id = EXCLUDED.cv_id, cv_updated_at = EXCLUDED.cv_updated_at,
			prompt_id = EXCLUDED.prompt_id, model = EXCLUDED.model, screen = EXCLUDED.screen, created_at = now()`,
		screen.JobID, screen.CVID, screen.CVUpdatedAt, screen.PromptID, screen.Model, screen.Screen)
	return err
}

// GetCVScreen returns the job's screen.
func (s *Store) GetCVScreen(ctx context.Context, jobID uuid.UUID) (CVScreen, error) {
	var screen CVScreen
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, cv_id, cv_updated_at, prompt_id, model, screen, created_at FROM cv_screens WHERE job_id = $1`, jobID).
		Scan(&screen.JobID, &screen.CVID, &screen.CVUpdatedAt, &screen.PromptID, &screen.Model, &screen.Screen, &screen.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CVScreen{}, ErrCVScreenNotFound
	}
	return screen, err
}

// ListCVsAwaitingScreen returns the tailored CVs of open pursued jobs that
// have no screen, or one older than the CV or than the prompt, the most
// recently changed first.
func (s *Store) ListCVsAwaitingScreen(ctx context.Context, promptID uuid.UUID, limit int) ([]CV, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+prefixedCVColumns+` FROM cvs
		JOIN jobs ON jobs.id = cvs.job_id AND jobs.closed_at IS NULL
		JOIN job_decisions ON job_decisions.job_id = jobs.id AND job_decisions.decision = 'pursue'
		LEFT JOIN cv_screens ON cv_screens.job_id = cvs.job_id
		WHERE cvs.kind = 'tailored'
		  AND (cv_screens.job_id IS NULL OR cv_screens.cv_updated_at < cvs.updated_at OR cv_screens.prompt_id <> $1)
		ORDER BY cvs.updated_at DESC LIMIT $2`, promptID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (CV, error) { return scanCV(row) })
}
