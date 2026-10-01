-- +goose Up
-- interview_prep writes each pursued job's interview pack: the likely
-- questions and the confirmed cases to tell. Its route starts on the model
-- job facts run on.
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix', 'recruiter_screen', 'market_gaps', 'interview_prep'));
INSERT INTO task_routes (kind, provider_id, model, fallback_provider_id, fallback_model)
SELECT 'interview_prep', provider_id, model, fallback_provider_id, fallback_model FROM task_routes WHERE kind = 'job_facts'
ON CONFLICT (kind) DO NOTHING;

-- One pack per job, written against the knowledge base as it was.
CREATE TABLE interview_packs (
    job_id          uuid PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    prompt_id       uuid NOT NULL REFERENCES agent_prompts (id),
    knowledge_hash  text NOT NULL,
    model           text NOT NULL,
    pack            jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE interview_packs;
DELETE FROM task_routes WHERE kind = 'interview_prep';
DELETE FROM agent_prompts WHERE kind = 'interview_prep';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix', 'recruiter_screen', 'market_gaps'));
