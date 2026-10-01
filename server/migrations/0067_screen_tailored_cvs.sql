-- +goose Up
-- A recruiter_screen reads a pursued job's tailored CV as the job's recruiter
-- would and lists the likely reasons to reject it. Its route starts on the
-- model job facts run on; the owner can move it in Settings.
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix', 'recruiter_screen'));
INSERT INTO task_routes (kind, provider_id, model, fallback_provider_id, fallback_model)
SELECT 'recruiter_screen', provider_id, model, fallback_provider_id, fallback_model FROM task_routes WHERE kind = 'job_facts'
ON CONFLICT (kind) DO NOTHING;

-- One screen per job, of the CV as it was when screened.
CREATE TABLE cv_screens (
    job_id        uuid PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    cv_id         uuid NOT NULL REFERENCES cvs (id) ON DELETE CASCADE,
    cv_updated_at timestamptz NOT NULL,
    prompt_id     uuid NOT NULL REFERENCES agent_prompts (id),
    model         text NOT NULL,
    screen        jsonb NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE cv_screens;
DELETE FROM task_routes WHERE kind = 'recruiter_screen';
DELETE FROM agent_prompts WHERE kind = 'recruiter_screen';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix'));
