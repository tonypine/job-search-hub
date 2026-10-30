-- +goose Up
-- The tokens an agent run's Claude Code session read and wrote, as its
-- result reports them, so a kind's numbers count them as a model call's.
-- A run finished before it reports none.
ALTER TABLE agent_runs
    ADD COLUMN prompt_tokens     integer NOT NULL DEFAULT 0,
    ADD COLUMN completion_tokens integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE agent_runs DROP COLUMN prompt_tokens, DROP COLUMN completion_tokens;
