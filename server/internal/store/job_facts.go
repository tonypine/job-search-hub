package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// JobFacts are the facts a model read from one job, in the shape the
// job_facts prompt version it followed asked for.
type JobFacts struct {
	JobID       uuid.UUID       `json:"job_id"`
	PromptID    uuid.UUID       `json:"prompt_id"`
	Model       string          `json:"model"`
	Facts       json.RawMessage `json:"facts"`
	ExtractedAt time.Time       `json:"extracted_at"`
}

// JobAwaitingFacts is an open job whose facts are missing or stale, with the
// hash of the text the new facts will be read from.
type JobAwaitingFacts struct {
	Job
	TextHash []byte
}

// jobTextHash is what job_facts.text_hash holds: the hash of the job text the
// facts cover.
const jobTextHash = `sha256(convert_to(jobs.title || E'\n' || jobs.location || E'\n' || jobs.description, 'UTF8'))`

// ListJobsAwaitingFacts returns up to limit open jobs with a description,
// newest first, that have no facts, facts from another prompt version, or
// facts read from text that has since changed. A job without a description,
// such as one read from an alert email, has nothing to read facts from.
func (s *Store) ListJobsAwaitingFacts(ctx context.Context, promptID uuid.UUID, limit int) ([]JobAwaitingFacts, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+prefixedJobColumns+`, `+jobTextHash+`
		FROM jobs LEFT JOIN job_facts ON job_facts.job_id = jobs.id
		WHERE jobs.closed_at IS NULL AND btrim(jobs.description) <> ''
		  AND (job_facts.job_id IS NULL OR job_facts.prompt_id <> $1 OR job_facts.text_hash <> `+jobTextHash+`)
		ORDER BY jobs.first_seen_at DESC
		LIMIT $2`, promptID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobAwaitingFacts, error) {
		var awaiting JobAwaitingFacts
		job, err := scanJob(row, &awaiting.TextHash)
		awaiting.Job = job
		return awaiting, err
	})
}

// NewJobFacts are facts just read from a job's text, whose hash is TextHash.
type NewJobFacts struct {
	JobID    uuid.UUID
	PromptID uuid.UUID
	Model    string
	TextHash []byte
	Facts    json.RawMessage
}

// SaveJobFacts replaces the job's facts. Facts are derived from the job, with
// their own provenance, so saving them records no change.
func (s *Store) SaveJobFacts(ctx context.Context, input NewJobFacts) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO job_facts (job_id, prompt_id, model, text_hash, facts)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (job_id) DO UPDATE SET
			prompt_id = EXCLUDED.prompt_id, model = EXCLUDED.model, text_hash = EXCLUDED.text_hash,
			facts = EXCLUDED.facts, extracted_at = now()`,
		input.JobID, input.PromptID, input.Model, input.TextHash, input.Facts)
	if isForeignKeyViolation(err) {
		return ErrJobNotFound
	}
	return err
}
