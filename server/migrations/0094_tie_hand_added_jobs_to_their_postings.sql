-- +goose Up
-- A job added by hand that was read from a Greenhouse, Lever or Ashby
-- posting names that posting: its provider, its board's token and the
-- posting's id there. When the hub keeps that board, later or already, its
-- sync makes this job the board's posting instead of adding a second one.
-- Empty for any other job.
ALTER TABLE jobs
    ADD COLUMN posting_provider text NOT NULL DEFAULT '',
    ADD COLUMN posting_board_token text NOT NULL DEFAULT '',
    ADD COLUMN posting_external_id text NOT NULL DEFAULT '';
CREATE INDEX jobs_hand_added_posting ON jobs (posting_provider, lower(posting_board_token))
    WHERE source = 'manual' AND posting_provider <> '';

-- Jobs added by hand from a provider's posting link before now name the
-- posting through that link.
WITH matched AS (
    SELECT id, provider, parts[1] AS board_token, parts[2] AS external_id
    FROM (
        SELECT id, 'greenhouse' AS provider, regexp_match(url, '^https?://(?:job-boards|boards)\.greenhouse\.io/([^/?#]+)/jobs/([^/?#]+)') AS parts
        FROM jobs WHERE source = 'manual'
        UNION ALL
        SELECT id, 'lever', regexp_match(url, '^https?://jobs\.lever\.co/([^/?#]+)/([^/?#]+)') FROM jobs WHERE source = 'manual'
        UNION ALL
        SELECT id, 'ashby', regexp_match(url, '^https?://jobs\.ashbyhq\.com/([^/?#]+)/([^/?#]+)') FROM jobs WHERE source = 'manual'
    ) candidates
    WHERE parts IS NOT NULL AND parts[2] <> 'embed'
)
UPDATE jobs SET posting_provider = matched.provider, posting_board_token = matched.board_token, posting_external_id = matched.external_id
FROM matched WHERE jobs.id = matched.id;

-- +goose Down
DROP INDEX jobs_hand_added_posting;
ALTER TABLE jobs DROP COLUMN posting_provider, DROP COLUMN posting_board_token, DROP COLUMN posting_external_id;
