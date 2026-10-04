package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AlertJobAwaitingText is an open alert job with no more of its posting than
// a snippet.
type AlertJobAwaitingText struct {
	Job         Job
	CompanyName string
	// Searched is whether Google for Jobs was asked for its text already.
	Searched bool
}

// ListAlertJobsAwaitingText returns the open, undismissed alert jobs whose
// text is no longer than an alert's snippet, newest first.
func (s *Store) ListAlertJobsAwaitingText(ctx context.Context) ([]AlertJobAwaitingText, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+prefixedJobColumns+`, COALESCE(companies.name, jobs.company_name),
		       EXISTS (SELECT 1 FROM posting_text_searches WHERE posting_text_searches.job_id = jobs.id)
		FROM jobs LEFT JOIN companies ON companies.id = jobs.company_id
		WHERE jobs.closed_at IS NULL AND jobs.dismissed_at IS NULL AND jobs.source = ANY($1)
		  AND octet_length(btrim(jobs.description)) < $2
		ORDER BY jobs.first_seen_at DESC, jobs.title`, AlertJobSources, AlertSnippetLength)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AlertJobAwaitingText, error) {
		var awaiting AlertJobAwaitingText
		job, err := scanJob(row, &awaiting.CompanyName, &awaiting.Searched)
		awaiting.Job = job
		return awaiting, err
	})
}

// CountPostingTextSearchesSince returns how many times Google for Jobs was
// asked for a posting's text since the given time.
func (s *Store) CountPostingTextSearchesSince(ctx context.Context, since time.Time) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM posting_text_searches WHERE searched_at >= $1`, since).Scan(&count)
	return count, err
}

// SavePostingText gives an alert job the posting's text found at sourceURL
// by a search at searchedAt. A job that got its text some other way since,
// as from its company's board, keeps that text.
func (s *Store) SavePostingText(ctx context.Context, actor Actor, jobID uuid.UUID, text, sourceURL string, searchedAt time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := recordPostingTextSearch(ctx, tx, jobID, searchedAt, true); err != nil {
			return err
		}
		var previousReason string
		err := tx.QueryRow(ctx, `
			SELECT text_missing_reason FROM jobs WHERE id = $1 AND source = ANY($2) AND octet_length(btrim(description)) < $3 FOR UPDATE`,
			jobID, AlertJobSources, AlertSnippetLength).Scan(&previousReason)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE jobs SET description = $2, text_missing_reason = '' WHERE id = $1`, jobID, text); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "job", entityID: jobID, operation: "update",
			before: map[string]string{"text_missing_reason": previousReason}, after: map[string]string{"description": "the posting's text"},
			sourceURL: sourceURL,
		})
	})
}

// RecordPostingTextMissing states why an alert job has no more than its
// snippet. searchedAt is when Google for Jobs was asked without finding it,
// and nil when it wasn't asked.
func (s *Store) RecordPostingTextMissing(ctx context.Context, jobID uuid.UUID, reason string, searchedAt *time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if searchedAt != nil {
			if err := recordPostingTextSearch(ctx, tx, jobID, *searchedAt, false); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE jobs SET text_missing_reason = $2 WHERE id = $1 AND text_missing_reason <> $2`, jobID, reason)
		return err
	})
}

func recordPostingTextSearch(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, searchedAt time.Time, found bool) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO posting_text_searches (job_id, searched_at, found) VALUES ($1, $2, $3)
		ON CONFLICT (job_id) DO UPDATE SET searched_at = EXCLUDED.searched_at, found = EXCLUDED.found`, jobID, searchedAt, found)
	return err
}
