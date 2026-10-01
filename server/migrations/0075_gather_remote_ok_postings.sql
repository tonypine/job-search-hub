-- +goose Up
-- Jobs gathered from Remote OK's feed.
ALTER TABLE jobs DROP CONSTRAINT jobs_source_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_check
    CHECK (source IN ('job_board', 'manual', 'himalayas', 'remoteok', 'indeed', 'linkedin', 'glassdoor', 'careers_page'));

-- +goose Down
DELETE FROM jobs WHERE source = 'remoteok';
ALTER TABLE jobs DROP CONSTRAINT jobs_source_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_check
    CHECK (source IN ('job_board', 'manual', 'himalayas', 'indeed', 'linkedin', 'glassdoor', 'careers_page'));
