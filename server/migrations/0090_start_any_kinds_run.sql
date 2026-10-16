-- +goose Up
-- A run the owner starts for a job or a company answers at once with the id
-- it will be recorded under. Agent runs run on the Mac, so their start is a
-- task: it carries that id, and the Claude model chosen for that run only,
-- empty to take the kind's route, until the Mac starts the run.
ALTER TABLE task_requests
    ADD COLUMN run_id uuid,
    ADD COLUMN model  text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE task_requests DROP COLUMN model, DROP COLUMN run_id;
