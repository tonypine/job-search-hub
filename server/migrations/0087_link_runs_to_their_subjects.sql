-- +goose Up
-- Every run names what it ran for: its subject's type and id. A model call
-- already kept the id; an agent run kept only its free-text input. A model
-- call also keeps its provider's name and whether the owner asked for it.
ALTER TABLE task_runs
    ADD COLUMN subject_type     text CHECK (subject_type IN ('job', 'company', 'mail_message', 'linkedin_conversation', 'profile')),
    ADD COLUMN provider         text NOT NULL DEFAULT '',
    ADD COLUMN started_by_owner boolean NOT NULL DEFAULT false;
ALTER TABLE agent_runs
    ADD COLUMN subject_type text CHECK (subject_type IN ('job', 'company', 'mail_message', 'linkedin_conversation', 'profile')),
    ADD COLUMN subject_id   uuid;

-- Claude wrote full briefs with the job_brief prompt; they are a kind of
-- their own now.
UPDATE task_runs SET kind = 'job_brief_full' WHERE kind = 'job_brief' AND base_url = 'claude-cli';

UPDATE task_runs SET subject_type = CASE
        WHEN kind IN ('job_facts', 'job_brief', 'job_brief_full', 'job_cv', 'recruiter_screen', 'interview_prep') THEN 'job'
        WHEN kind = 'mail_triage' THEN 'mail_message'
        WHEN kind = 'linkedin_conversation' THEN 'linkedin_conversation'
    END
WHERE subject_id IS NOT NULL;

-- A run's provider by its address: Claude's CLI, a provider at that address,
-- or the hub's own runtime, which listens on the loopback.
UPDATE task_runs SET provider = CASE
    WHEN base_url = 'claude-cli' THEN 'Claude'
    ELSE coalesce(
        (SELECT name FROM model_providers
         WHERE kind = 'openai_compatible' AND rtrim(base_url, '/') = rtrim(task_runs.base_url, '/') ORDER BY created_at LIMIT 1),
        CASE WHEN base_url = 'hub runtime' OR base_url ~ '^http://127\.0\.0\.1:[0-9]+/v1$' THEN
            (SELECT name FROM model_providers WHERE kind = 'hub_runtime' ORDER BY created_at LIMIT 1)
        END,
        '')
    END;

-- A job fix's input is its job's id.
UPDATE agent_runs SET subject_type = 'job', subject_id = jobs.id
FROM jobs
WHERE agent_runs.kind = 'job_fix'
  AND jobs.id = CASE WHEN agent_runs.input ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN agent_runs.input::uuid END;

UPDATE agent_runs SET subject_type = 'profile' WHERE kind = 'profile_seed';

-- A company run's input names its company by id, domain, URL or name: the
-- company it names, matched as the prompt matches it, or else the first
-- company the run changed.
UPDATE agent_runs SET subject_id = matched.company_id, subject_type = 'company'
FROM (
    SELECT runs.id, coalesce(
        (SELECT companies.id FROM companies
         WHERE companies.id = CASE WHEN btrim(runs.input) ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                                   THEN btrim(runs.input)::uuid END),
        (SELECT companies.id FROM companies
         WHERE companies.domain = regexp_replace(rtrim(regexp_replace(regexp_replace(lower(btrim(runs.input)),
                                      '^[a-z][a-z0-9+.-]*://', ''), '[/?#:].*$', ''), '.'), '^www\.', '')),
        (SELECT CASE WHEN count(*) = 1 THEN (array_agg(companies.id))[1] END FROM companies
         WHERE lower(companies.name) = lower(btrim(runs.input))),
        (SELECT changes.entity_id FROM changes
         WHERE changes.agent_run_id = runs.id AND changes.entity_type = 'company' ORDER BY changes.id LIMIT 1)
    ) AS company_id
    FROM agent_runs runs
    WHERE runs.kind IN ('company_triage', 'job_finder')
) matched
WHERE agent_runs.id = matched.id AND matched.company_id IS NOT NULL;

CREATE INDEX task_runs_subject ON task_runs (subject_id, started_at DESC) WHERE subject_id IS NOT NULL;
CREATE INDEX agent_runs_started ON agent_runs (started_at DESC);
CREATE INDEX agent_runs_subject ON agent_runs (subject_id, started_at DESC) WHERE subject_id IS NOT NULL;

-- +goose Down
DROP INDEX agent_runs_subject;
DROP INDEX agent_runs_started;
DROP INDEX task_runs_subject;
UPDATE task_runs SET kind = 'job_brief' WHERE kind = 'job_brief_full';
ALTER TABLE agent_runs DROP COLUMN subject_id, DROP COLUMN subject_type;
ALTER TABLE task_runs DROP COLUMN started_by_owner, DROP COLUMN provider, DROP COLUMN subject_type;
