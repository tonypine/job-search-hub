-- +goose Up
-- What a conversation someone else started is: a recruiter reaching out,
-- someone the owner knows, a sales pitch, or something else; and, for a
-- recruiter, the company hiring and the role.
ALTER TABLE linkedin_conversations
    ADD COLUMN classification text NOT NULL DEFAULT ''
        CHECK (classification IN ('', 'recruiter_outreach', 'known_person', 'sales_pitch', 'other')),
    ADD COLUMN classified_by text NOT NULL DEFAULT '' CHECK (classified_by IN ('', 'rule', 'model')),
    ADD COLUMN classification_reason text NOT NULL DEFAULT '',
    ADD COLUMN classification_prompt_id uuid REFERENCES agent_prompts (id),
    ADD COLUMN classified_at timestamptz,
    ADD COLUMN hiring_company text NOT NULL DEFAULT '',
    ADD COLUMN role text NOT NULL DEFAULT '',
    ADD COLUMN is_agency boolean NOT NULL DEFAULT false;

ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage',
                    'linkedin_conversation'));

-- +goose Down
DELETE FROM agent_prompts WHERE kind = 'linkedin_conversation';
ALTER TABLE agent_prompts DROP CONSTRAINT agent_prompts_kind_check;
ALTER TABLE agent_prompts ADD CONSTRAINT agent_prompts_kind_check
    CHECK (kind IN ('company_triage', 'job_facts', 'company_session', 'job_session', 'outreach_draft', 'mail_triage'));
ALTER TABLE linkedin_conversations DROP COLUMN is_agency, DROP COLUMN role, DROP COLUMN hiring_company, DROP COLUMN classified_at,
    DROP COLUMN classification_prompt_id, DROP COLUMN classification_reason, DROP COLUMN classified_by, DROP COLUMN classification;
