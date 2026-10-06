-- +goose Up
ALTER TABLE companies ADD COLUMN industry text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE companies DROP COLUMN industry;
