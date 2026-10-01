package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrInterviewPackNotFound = errors.New("interview pack not found")

// InterviewPack is a pursued job's interview preparation, in the shape the
// interview_prep prompt's schema gives, written against the knowledge base
// as KnowledgeHash records it.
type InterviewPack struct {
	JobID         uuid.UUID       `json:"job_id"`
	PromptID      uuid.UUID       `json:"prompt_id"`
	KnowledgeHash string          `json:"-"`
	Model         string          `json:"model"`
	Pack          json.RawMessage `json:"pack"`
	CreatedAt     time.Time       `json:"created_at"`
}

// SaveInterviewPack keeps the job's pack, replacing an earlier one.
func (s *Store) SaveInterviewPack(ctx context.Context, pack InterviewPack) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO interview_packs (job_id, prompt_id, knowledge_hash, model, pack) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (job_id) DO UPDATE SET prompt_id = EXCLUDED.prompt_id, knowledge_hash = EXCLUDED.knowledge_hash,
			model = EXCLUDED.model, pack = EXCLUDED.pack, created_at = now()`,
		pack.JobID, pack.PromptID, pack.KnowledgeHash, pack.Model, pack.Pack)
	return err
}

// GetInterviewPack returns the job's pack.
func (s *Store) GetInterviewPack(ctx context.Context, jobID uuid.UUID) (InterviewPack, error) {
	var pack InterviewPack
	err := s.pool.QueryRow(ctx, `SELECT job_id, prompt_id, knowledge_hash, model, pack, created_at FROM interview_packs WHERE job_id = $1`, jobID).
		Scan(&pack.JobID, &pack.PromptID, &pack.KnowledgeHash, &pack.Model, &pack.Pack, &pack.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return InterviewPack{}, ErrInterviewPackNotFound
	}
	return pack, err
}

// ListJobsAwaitingInterviewPack returns the open pursued jobs with no pack,
// or one written with another prompt or against an older knowledge base,
// the most recently pursued first.
func (s *Store) ListJobsAwaitingInterviewPack(ctx context.Context, promptID uuid.UUID, knowledgeHash string, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT jobs.id FROM jobs
		JOIN job_decisions ON job_decisions.job_id = jobs.id AND job_decisions.decision = 'pursue'
		LEFT JOIN interview_packs ON interview_packs.job_id = jobs.id
		WHERE jobs.closed_at IS NULL
		  AND (interview_packs.job_id IS NULL OR interview_packs.prompt_id <> $1 OR interview_packs.knowledge_hash <> $2)
		ORDER BY job_decisions.decided_at DESC LIMIT $3`, promptID, knowledgeHash, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}
