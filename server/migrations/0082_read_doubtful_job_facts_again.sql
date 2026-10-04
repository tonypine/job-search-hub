-- +goose Up
-- Facts read again by a stronger model because the first reading was
-- doubtful keep why it was, and which model read them first.
ALTER TABLE job_facts ADD COLUMN doubt text NOT NULL DEFAULT '', ADD COLUMN first_model text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE job_facts DROP COLUMN doubt, DROP COLUMN first_model;
