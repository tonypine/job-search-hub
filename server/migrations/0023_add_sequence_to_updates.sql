-- +goose Up
-- The order updates were recorded in, which the event stream resumes from.
ALTER TABLE updates ADD COLUMN sequence bigserial;
CREATE UNIQUE INDEX updates_sequence ON updates (sequence);

-- +goose Down
ALTER TABLE updates DROP COLUMN sequence;
