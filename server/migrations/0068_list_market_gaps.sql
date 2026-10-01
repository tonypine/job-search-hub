-- +goose Up
-- market_gaps plans how to close the skills good fits keep asking for and
-- the knowledge base lacks. Its route starts on the model job facts run on.
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix', 'recruiter_screen', 'market_gaps'));
INSERT INTO task_routes (kind, provider_id, model, fallback_provider_id, fallback_model)
SELECT 'market_gaps', provider_id, model, fallback_provider_id, fallback_model FROM task_routes WHERE kind = 'job_facts'
ON CONFLICT (kind) DO NOTHING;

-- The latest gaps, replaced whole on each refresh.
CREATE TABLE market_gaps (
    technology   text PRIMARY KEY,
    job_count    integer NOT NULL,
    good_fits    integer NOT NULL,
    job_ids      uuid[] NOT NULL,
    plan_kind    text NOT NULL DEFAULT '',
    plan         text NOT NULL DEFAULT '',
    computed_at  timestamptz NOT NULL
);

-- +goose Down
DROP TABLE market_gaps;
DELETE FROM task_routes WHERE kind = 'market_gaps';
DELETE FROM agent_prompts WHERE kind = 'market_gaps';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix', 'recruiter_screen'));
