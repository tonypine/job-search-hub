package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListReadIndexPages returns the pages of a crawl's index already read for a
// URL pattern.
func (s *Store) ListReadIndexPages(ctx context.Context, crawlID, urlPattern string) (map[int]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT page FROM board_index_pages WHERE crawl_id = $1 AND url_pattern = $2`, crawlID, urlPattern)
	if err != nil {
		return nil, err
	}
	pages, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, err
	}
	read := make(map[int]bool, len(pages))
	for _, page := range pages {
		read[page] = true
	}
	return read, nil
}

// RecordIndexPage records that a page of a crawl's index was read for a URL
// pattern.
func (s *Store) RecordIndexPage(ctx context.Context, crawlID, urlPattern string, page int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO board_index_pages (crawl_id, url_pattern, page) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, crawlID, urlPattern, page)
	return err
}

// SaveDiscoveredBoardTokens stores the board tokens an index listed for a
// provider, and returns how many were new.
func (s *Store) SaveDiscoveredBoardTokens(ctx context.Context, provider string, boardTokens []string) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO discovered_board_tokens (provider, board_token) SELECT $1, unnest($2::text[]) ON CONFLICT DO NOTHING`, provider, boardTokens)
	return int(tag.RowsAffected()), err
}

// ListUncheckedBoardTokens returns up to limit of a provider's discovered
// tokens whose titles weren't checked yet, leaving out the boards already
// stored.
func (s *Store) ListUncheckedBoardTokens(ctx context.Context, provider string, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT board_token FROM discovered_board_tokens
		WHERE provider = $1 AND checked_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM job_boards WHERE job_boards.provider = $1 AND job_boards.board_token = discovered_board_tokens.board_token)
		ORDER BY found_at, board_token
		LIMIT $2`, provider, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// RecordBoardTokenCheck records that a discovered token's titles were
// checked, with the board kept for it, or nil when none was.
func (s *Store) RecordBoardTokenCheck(ctx context.Context, provider, boardToken string, jobBoardID *uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE discovered_board_tokens SET checked_at = now(), job_board_id = $3 WHERE provider = $1 AND board_token = $2`,
		provider, boardToken, jobBoardID)
	return err
}
