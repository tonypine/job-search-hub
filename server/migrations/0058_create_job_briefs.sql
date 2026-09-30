-- +goose Up
-- A brief per job and tier: the local model's pre-brief, and Claude's full
-- brief for the best matches. Each strength and weakness cites the knowledge
-- base entries it rests on; knowledge_hash is the knowledge base, profile and
-- criteria it was written against, so a brief can be found stale.
CREATE TABLE job_briefs (
    job_id         uuid NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    tier           text NOT NULL CHECK (tier IN ('pre', 'full')),
    prompt_id      uuid NOT NULL REFERENCES agent_prompts (id),
    model          text NOT NULL,
    match          text NOT NULL CHECK (match IN ('strong', 'possible', 'stretch', 'mismatch')),
    reason         text NOT NULL,
    strengths      jsonb NOT NULL DEFAULT '[]',
    weaknesses     jsonb NOT NULL DEFAULT '[]',
    knowledge_hash text NOT NULL,
    written_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, tier)
);

-- +goose Down
DROP TABLE job_briefs;
