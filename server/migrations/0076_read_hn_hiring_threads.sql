-- +goose Up
-- Hacker News' monthly "Who is hiring?" thread, read comment by comment by
-- the local model through the hiring_thread prompt.
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix', 'recruiter_screen', 'market_gaps', 'interview_prep', 'hiring_thread'));
INSERT INTO task_routes (kind, provider_id, model, fallback_provider_id, fallback_model)
SELECT 'hiring_thread', provider_id, model, fallback_provider_id, fallback_model FROM task_routes WHERE kind = 'job_facts'
ON CONFLICT (kind) DO NOTHING;

ALTER TABLE jobs DROP CONSTRAINT jobs_source_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_check
    CHECK (source IN ('job_board', 'manual', 'himalayas', 'remoteok', 'hackernews', 'indeed', 'linkedin', 'glassdoor', 'careers_page'));

CREATE TABLE hiring_thread_comments (
    comment_id bigint PRIMARY KEY,
    thread_id  bigint NOT NULL,
    job_count  integer NOT NULL,
    prompt_id  uuid REFERENCES agent_prompts (id),
    read_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE hiring_thread_comments;
DELETE FROM jobs WHERE source = 'hackernews';
ALTER TABLE jobs DROP CONSTRAINT jobs_source_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_check
    CHECK (source IN ('job_board', 'manual', 'himalayas', 'remoteok', 'indeed', 'linkedin', 'glassdoor', 'careers_page'));
DELETE FROM task_routes WHERE kind = 'hiring_thread';
DELETE FROM agent_prompts WHERE kind = 'hiring_thread';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation', 'recruiter_reply', 'profile_audit', 'job_finder', 'profile_seed', 'profile_interview',
                    'job_brief', 'job_cv', 'job_fix', 'recruiter_screen', 'market_gaps', 'interview_prep'));
