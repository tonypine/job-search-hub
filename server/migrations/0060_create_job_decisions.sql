-- +goose Up
-- The owner's latest verdict on a job: pursue it, skip it (a dismissal), or
-- look again later. The change log keeps the ones before.
CREATE TABLE job_decisions (
    job_id     uuid PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    decision   text NOT NULL CHECK (decision IN ('pursue', 'skip', 'later')),
    reason     text NOT NULL DEFAULT '',
    decided_at timestamptz NOT NULL DEFAULT now()
);
-- Jobs dismissed before decisions existed read as skipped.
INSERT INTO job_decisions (job_id, decision, reason, decided_at)
SELECT id, 'skip', dismissal_reason, dismissed_at FROM jobs WHERE dismissed_at IS NOT NULL;

-- +goose Down
DROP TABLE job_decisions;
