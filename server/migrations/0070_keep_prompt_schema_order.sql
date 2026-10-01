-- +goose Up
-- A prompt's result schema is kept as written: jsonb sorts object keys, and
-- a model fills an answer's fields in the order its schema lists them.
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_result_schema_check;
ALTER TABLE agent_prompts ALTER COLUMN result_schema TYPE json USING result_schema::text::json;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_result_schema_check CHECK (result_schema IS NULL OR json_typeof(result_schema) = 'object');

-- +goose Down
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_result_schema_check;
ALTER TABLE agent_prompts ALTER COLUMN result_schema TYPE jsonb USING result_schema::jsonb;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_result_schema_check CHECK (result_schema IS NULL OR jsonb_typeof(result_schema) = 'object');
