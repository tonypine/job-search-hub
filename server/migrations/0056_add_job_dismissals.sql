-- +goose Up
-- The owner dismisses a job they don't want to see again, with an optional
-- reason. Syncs never clear it, so a job listed again stays dismissed.
ALTER TABLE jobs
    ADD COLUMN dismissed_at     timestamptz,
    ADD COLUMN dismissal_reason text NOT NULL DEFAULT '';
DROP INDEX jobs_open_first_seen;
CREATE INDEX jobs_open_first_seen ON jobs (first_seen_at DESC) WHERE closed_at IS NULL AND dismissed_at IS NULL;

-- +goose Down
DROP INDEX jobs_open_first_seen;
CREATE INDEX jobs_open_first_seen ON jobs (first_seen_at DESC) WHERE closed_at IS NULL;
ALTER TABLE jobs DROP COLUMN dismissal_reason, DROP COLUMN dismissed_at;
