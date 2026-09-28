-- +goose Up
ALTER TABLE companies ADD COLUMN found_via text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE companies DROP COLUMN found_via;
