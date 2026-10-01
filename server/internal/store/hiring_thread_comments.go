package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListReadHiringComments returns the ids of the hiring thread's comments
// already read.
func (s *Store) ListReadHiringComments(ctx context.Context, threadID int64) (map[int64]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT comment_id FROM hiring_thread_comments WHERE thread_id = $1`, threadID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, err
	}
	read := make(map[int64]bool, len(ids))
	for _, id := range ids {
		read[id] = true
	}
	return read, nil
}

// RecordHiringComment records that a hiring thread's comment was read and
// how many jobs it gave; promptID is nil when code left it out unread.
func (s *Store) RecordHiringComment(ctx context.Context, threadID, commentID int64, jobCount int, promptID *uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO hiring_thread_comments (comment_id, thread_id, job_count, prompt_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (comment_id) DO UPDATE SET job_count = EXCLUDED.job_count, prompt_id = EXCLUDED.prompt_id, read_at = now()`,
		commentID, threadID, jobCount, promptID)
	return err
}
