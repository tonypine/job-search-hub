-- +goose Up
-- Worked examples a prompt version shows the model before the real input,
-- each an input and the answer it should give, sent as earlier chat turns.
ALTER TABLE agent_prompts ADD COLUMN examples jsonb NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE agent_prompts DROP COLUMN examples;
