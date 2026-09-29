-- +goose Up
-- A Claude session can be about the owner's profile, not a company or a job:
-- the interview that deepens the knowledge base, with its own prompt kind.
ALTER TABLE claude_sessions ADD COLUMN about_profile boolean NOT NULL DEFAULT false;
ALTER TABLE claude_sessions DROP CONSTRAINT claude_sessions_check;
ALTER TABLE claude_sessions ADD CONSTRAINT claude_sessions_subject_check
    CHECK (num_nonnulls(company_id, job_id) + about_profile::int = 1);

ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview'));

-- +goose Down
DELETE FROM agent_prompts WHERE kind = 'profile_interview';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed'));
DELETE FROM claude_sessions WHERE about_profile;
ALTER TABLE claude_sessions DROP CONSTRAINT claude_sessions_subject_check;
ALTER TABLE claude_sessions ADD CONSTRAINT claude_sessions_check CHECK ((company_id IS NULL) <> (job_id IS NULL));
ALTER TABLE claude_sessions DROP COLUMN about_profile;
