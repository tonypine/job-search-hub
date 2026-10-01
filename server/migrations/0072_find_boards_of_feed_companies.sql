-- +goose Up
-- A board found for a feed job's company may belong to no company the hub
-- keeps: it carries the company's name, and ties to the hub's company of that
-- name when one exists.
ALTER TABLE job_boards
    ALTER COLUMN company_id DROP NOT NULL,
    ADD COLUMN company_name text NOT NULL DEFAULT '',
    ADD CONSTRAINT job_boards_company_check CHECK (company_id IS NOT NULL OR company_name <> '');

-- One search per company name. searched_providers are the providers that
-- answered without a board, so a search a provider couldn't answer resumes
-- with that provider alone, and a name searched on every provider waits
-- until searched_at is old.
CREATE TABLE board_searches (
    company_key        text PRIMARY KEY CHECK (company_key <> ''),
    company_name       text NOT NULL,
    searched_at        timestamptz NOT NULL,
    searched_providers text[] NOT NULL DEFAULT '{}',
    job_board_id       uuid REFERENCES job_boards (id) ON DELETE SET NULL
);

-- +goose Down
DROP TABLE board_searches;
DELETE FROM jobs WHERE job_board_id IN (SELECT id FROM job_boards WHERE company_id IS NULL);
DELETE FROM job_boards WHERE company_id IS NULL;
ALTER TABLE job_boards
    DROP CONSTRAINT job_boards_company_check,
    DROP COLUMN company_name,
    ALTER COLUMN company_id SET NOT NULL;
