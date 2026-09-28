-- +goose Up
-- What kind of mail a received message is, and whether a rule or the model
-- said so. An empty classification is a message not read yet.
ALTER TABLE mail_messages
    ADD COLUMN classification text NOT NULL DEFAULT ''
        CHECK (classification IN ('', 'human_reply', 'application_confirmation', 'rejection', 'interview_invite',
                                  'recruiter_outreach', 'job_alert', 'noise')),
    ADD COLUMN classified_by text NOT NULL DEFAULT '' CHECK (classified_by IN ('', 'rule', 'model')),
    ADD COLUMN classification_reason text NOT NULL DEFAULT '',
    ADD COLUMN classification_prompt_id uuid REFERENCES agent_prompts (id),
    ADD COLUMN classified_at timestamptz;

CREATE INDEX mail_messages_awaiting_classification ON mail_messages (sent_at DESC)
    WHERE classification = '' AND direction = 'received';

ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage'));

-- +goose Down
DELETE FROM agent_prompts WHERE kind = 'mail_triage';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft'));
DROP INDEX mail_messages_awaiting_classification;
ALTER TABLE mail_messages DROP COLUMN classified_at, DROP COLUMN classification_prompt_id, DROP COLUMN classification_reason,
    DROP COLUMN classified_by, DROP COLUMN classification;
