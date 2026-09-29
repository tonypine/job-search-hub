-- +goose Up
-- The text read from each uploaded file once, at upload, for the agents; or
-- why none could be read.
ALTER TABLE artifacts
    ADD COLUMN text text,
    ADD COLUMN text_error text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE artifacts DROP COLUMN text_error, DROP COLUMN text;
