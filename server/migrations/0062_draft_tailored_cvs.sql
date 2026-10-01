-- +goose Up
-- The job_cv prompt has Claude tailor the base CV to a pursued job. A job
-- keeps one tailored CV; a new draft replaces it.
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv'));
CREATE UNIQUE INDEX cvs_one_per_job ON cvs (job_id) WHERE kind = 'tailored';

-- +goose Down
DROP INDEX cvs_one_per_job;
DELETE FROM agent_prompts WHERE kind = 'job_cv';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief'));
