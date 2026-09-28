-- +goose Up
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check CHECK (kind IN ('company_triage', 'job_facts'));
ALTER TABLE agent_prompts ADD COLUMN result_schema jsonb CHECK (result_schema IS NULL OR jsonb_typeof(result_schema) = 'object');

-- +goose Down
DELETE FROM agent_prompts WHERE kind = 'job_facts';
ALTER TABLE agent_prompts DROP COLUMN result_schema;
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check CHECK (kind IN ('company_triage'));
