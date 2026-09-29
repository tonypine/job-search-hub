-- +goose Up
-- Jobs read from the job alert emails Indeed, LinkedIn and Glassdoor send
-- the owner, and which alert emails have been read for jobs.
ALTER TABLE jobs DROP CONSTRAINT jobs_source_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_check
    CHECK (source IN ('job_board', 'manual', 'himalayas', 'indeed', 'linkedin', 'glassdoor'));

ALTER TABLE mail_messages
    ADD COLUMN alert_jobs_read_at timestamptz,
    ADD COLUMN alert_jobs_found integer;

-- +goose Down
ALTER TABLE mail_messages DROP COLUMN alert_jobs_found, DROP COLUMN alert_jobs_read_at;
DELETE FROM jobs WHERE source IN ('indeed', 'linkedin', 'glassdoor');
ALTER TABLE jobs DROP CONSTRAINT jobs_source_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_check CHECK (source IN ('job_board', 'manual', 'himalayas'));
