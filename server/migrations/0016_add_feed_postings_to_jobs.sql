-- +goose Up
-- Jobs gathered from job feeds such as Himalayas: no board of their own, a
-- company that may not be in the hub, and an expiry date.
ALTER TABLE jobs DROP CONSTRAINT jobs_source_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_check CHECK (source IN ('job_board', 'manual', 'himalayas'));
ALTER TABLE jobs
    ADD COLUMN company_name text NOT NULL DEFAULT '',
    ADD COLUMN expires_at   timestamptz;
CREATE UNIQUE INDEX jobs_feed_posting ON jobs (source, external_id) WHERE job_board_id IS NULL AND external_id IS NOT NULL;

-- +goose Down
DROP INDEX jobs_feed_posting;
ALTER TABLE jobs DROP COLUMN company_name, DROP COLUMN expires_at;
ALTER TABLE jobs DROP CONSTRAINT jobs_source_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_check CHECK (source IN ('job_board', 'manual'));
