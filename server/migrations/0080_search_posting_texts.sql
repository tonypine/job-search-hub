-- +goose Up
-- Why an alert job has no more of its posting than the alert gave; empty
-- once it has its text, or before anyone looked.
ALTER TABLE jobs ADD COLUMN text_missing_reason text NOT NULL DEFAULT '';

-- One Google for Jobs request per alert job, made once its company's board
-- had its chance. The rows of a month are its requests, held under the
-- plan's monthly quota.
CREATE TABLE posting_text_searches (
    job_id      uuid PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    searched_at timestamptz NOT NULL,
    found       boolean NOT NULL
);
CREATE INDEX posting_text_searches_searched_at ON posting_text_searches (searched_at);

-- +goose Down
DROP TABLE posting_text_searches;
ALTER TABLE jobs DROP COLUMN text_missing_reason;
