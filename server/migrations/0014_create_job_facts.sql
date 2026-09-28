-- +goose Up
-- The facts a model read from a job, per the job_facts prompt version it
-- followed. text_hash covers the text they were read from, so a job whose
-- title, location or description changes is read again.
CREATE TABLE job_facts (
    job_id       uuid PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    prompt_id    uuid NOT NULL REFERENCES agent_prompts (id),
    model        text NOT NULL CHECK (model <> ''),
    text_hash    bytea NOT NULL,
    facts        jsonb NOT NULL CHECK (jsonb_typeof(facts) = 'object'),
    extracted_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE job_facts;
