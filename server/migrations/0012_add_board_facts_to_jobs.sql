-- +goose Up
ALTER TABLE jobs
    ADD COLUMN pay             jsonb,
    ADD COLUMN employment_type text NOT NULL DEFAULT '',
    ADD COLUMN department      text NOT NULL DEFAULT '',
    ADD COLUMN other_locations text[] NOT NULL DEFAULT '{}',
    ADD COLUMN published_at    timestamptz;

-- +goose Down
ALTER TABLE jobs
    DROP COLUMN pay,
    DROP COLUMN employment_type,
    DROP COLUMN department,
    DROP COLUMN other_locations,
    DROP COLUMN published_at;
