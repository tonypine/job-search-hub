-- +goose Up
-- The card a pursue put on the pipeline, so taking the pursue back takes the
-- card off again while nothing has changed it since. A pursue of a job
-- already on the pipeline added none.
ALTER TABLE job_decisions ADD COLUMN added_application_id uuid REFERENCES applications (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE job_decisions DROP COLUMN added_application_id;
