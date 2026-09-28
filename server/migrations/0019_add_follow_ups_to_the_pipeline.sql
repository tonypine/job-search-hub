-- +goose Up
-- A phase can ask for a follow-up a number of days after a card enters it or
-- was last followed up.
ALTER TABLE pipeline_phases ADD COLUMN follow_up_days integer CHECK (follow_up_days IS NULL OR follow_up_days > 0);
ALTER TABLE applications ADD COLUMN last_followed_up_at timestamptz;

UPDATE pipeline_phases SET follow_up_days = 7 WHERE lower(name) = 'applied';
UPDATE pipeline_phases SET follow_up_days = 5 WHERE lower(name) IN ('in contact', 'interviewing');
UPDATE pipeline_phases SET follow_up_days = 3 WHERE lower(name) = 'offer';

-- +goose Down
ALTER TABLE applications DROP COLUMN last_followed_up_at;
ALTER TABLE pipeline_phases DROP COLUMN follow_up_days;
