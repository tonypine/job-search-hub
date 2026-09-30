-- +goose Up
-- The job_brief prompt writes each job's brief. Its route starts on the model
-- job facts run on; the owner can move it in Settings.
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief'));
INSERT INTO task_routes (kind, provider_id, model, fallback_provider_id, fallback_model)
SELECT 'job_brief', provider_id, model, fallback_provider_id, fallback_model FROM task_routes WHERE kind = 'job_facts'
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DELETE FROM task_routes WHERE kind = 'job_brief';
DELETE FROM job_briefs WHERE prompt_id IN (SELECT id FROM agent_prompts WHERE kind = 'job_brief');
DELETE FROM agent_prompts WHERE kind = 'job_brief';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview'));
