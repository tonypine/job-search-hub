-- +goose Up
-- When a job judged a strong match is still fresh the hub tells the owner
-- once; this is when it told them.
ALTER TABLE jobs ADD COLUMN fresh_match_told_at timestamptz;

-- +goose Down
ALTER TABLE jobs DROP COLUMN fresh_match_told_at;
