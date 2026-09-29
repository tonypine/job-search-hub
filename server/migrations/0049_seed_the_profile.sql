-- +goose Up
-- A profile_seed agent run turns the CV, the LinkedIn export and the answers
-- library into knowledge-base entries for the owner to confirm.
ALTER TABLE agent_runs DROP CONSTRAINT agent_runs_kind_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_kind_check CHECK (kind IN ('company_triage', 'job_finder', 'profile_seed'));

ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed'));

-- +goose Down
DELETE FROM agent_prompts WHERE kind = 'profile_seed';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder'));
DELETE FROM agent_runs WHERE kind = 'profile_seed';
ALTER TABLE agent_runs DROP CONSTRAINT agent_runs_kind_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_kind_check CHECK (kind IN ('company_triage', 'job_finder'));
