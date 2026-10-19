-- +goose Up
-- What a company does in one to three words, e.g. Payments, shown with its
-- size under its name. Empty until a triage run sets it.
ALTER TABLE companies ADD COLUMN industry text NOT NULL DEFAULT '';

-- The company triage prompt lives only in the owner's database, so its next
-- version asks for the industry, when there is a prompt to extend. The owner
-- sees it as a new version, and can save the previous body back.
WITH latest AS (
    SELECT kind, version, body, result_schema, examples FROM agent_prompts
    WHERE kind = 'company_triage' ORDER BY version DESC LIMIT 1
), added AS (
    INSERT INTO agent_prompts (kind, version, body, result_schema, examples, note)
    SELECT kind, version + 1,
           rtrim(body, E' \n') || E'\n\n## Industry\n\nSet the company''s industry with update_company: what it does in one to three words, such as Payments or Health data.\n',
           result_schema, examples, 'Asks for the company''s industry'
    FROM latest
    RETURNING id, kind, version, note
)
INSERT INTO changes (actor_kind, entity_type, entity_id, operation, after)
SELECT 'system', 'agent_prompt', id, 'create', jsonb_build_object('kind', kind, 'version', version, 'note', note)
FROM added;

-- +goose Down
DELETE FROM agent_prompts WHERE kind = 'company_triage' AND note = 'Asks for the company''s industry';
ALTER TABLE companies DROP COLUMN industry;
