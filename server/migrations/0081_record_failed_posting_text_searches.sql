-- +goose Up
-- A Google for Jobs request that failed still counts toward the month's
-- quota, and its job isn't asked about again.
ALTER TABLE posting_text_searches ADD COLUMN failed boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE posting_text_searches DROP COLUMN failed;
