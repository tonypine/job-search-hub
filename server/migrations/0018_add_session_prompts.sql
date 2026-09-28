-- +goose Up
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session'));

-- +goose Down
DELETE FROM agent_prompts WHERE kind IN ('company_session', 'job_session');
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check CHECK (kind IN ('company_triage', 'job_facts'));
