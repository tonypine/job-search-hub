-- +goose Up
ALTER TABLE agent_runs ADD COLUMN agent_prompt_version integer;

-- +goose Down
ALTER TABLE agent_runs DROP COLUMN agent_prompt_version;
