-- +goose Up
-- A job_fix agent run corrects a job's details from the owner's note; a
-- fix_job task asks the Mac to run one.
ALTER TABLE agent_runs DROP CONSTRAINT agent_runs_kind_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_kind_check CHECK (kind IN ('company_triage', 'job_finder', 'profile_seed', 'job_fix'));

ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix'));

ALTER TABLE task_requests DROP CONSTRAINT task_requests_kind_check;
ALTER TABLE task_requests ADD CONSTRAINT task_requests_kind_check CHECK (kind IN ('find_jobs', 'research_company', 'fix_job'));
ALTER TABLE task_requests ADD COLUMN job_id uuid REFERENCES jobs (id) ON DELETE CASCADE;

-- +goose Down
DELETE FROM task_requests WHERE kind = 'fix_job';
ALTER TABLE task_requests DROP COLUMN job_id;
ALTER TABLE task_requests DROP CONSTRAINT task_requests_kind_check;
ALTER TABLE task_requests ADD CONSTRAINT task_requests_kind_check CHECK (kind IN ('find_jobs', 'research_company'));

DELETE FROM agent_prompts WHERE kind = 'job_fix';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv'));

DELETE FROM agent_runs WHERE kind = 'job_fix';
ALTER TABLE agent_runs DROP CONSTRAINT agent_runs_kind_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_kind_check CHECK (kind IN ('company_triage', 'job_finder', 'profile_seed'));
