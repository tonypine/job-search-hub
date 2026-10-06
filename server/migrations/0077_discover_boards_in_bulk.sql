-- +goose Up
-- Boards found in bulk from public indexes, read at most daily.
ALTER TABLE job_boards
    ADD COLUMN found_by       text NOT NULL DEFAULT '' CHECK (found_by IN ('', 'search', 'discovery')),
    ADD COLUMN last_polled_at timestamptz;
UPDATE job_boards SET found_by = 'search' WHERE company_name <> '';

-- An index page read, so a discovery pass resumes where the last stopped.
CREATE TABLE board_index_pages (
    crawl_id    text NOT NULL,
    url_pattern text NOT NULL,
    page        integer NOT NULL,
    read_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (crawl_id, url_pattern, page)
);

-- A board token an index listed, and whether its titles name a role.
CREATE TABLE discovered_board_tokens (
    provider     text NOT NULL,
    board_token  text NOT NULL,
    found_at     timestamptz NOT NULL DEFAULT now(),
    checked_at   timestamptz,
    job_board_id uuid REFERENCES job_boards (id) ON DELETE SET NULL,
    PRIMARY KEY (provider, board_token)
);

-- +goose Down
DROP TABLE discovered_board_tokens;
DROP TABLE board_index_pages;
DELETE FROM jobs WHERE job_board_id IN (SELECT id FROM job_boards WHERE found_by = 'discovery');
DELETE FROM job_boards WHERE found_by = 'discovery';
ALTER TABLE job_boards DROP COLUMN last_polled_at, DROP COLUMN found_by;
