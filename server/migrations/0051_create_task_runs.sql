-- +goose Up
-- Every request the hub makes to a model, whatever its outcome: what it was
-- for, which model answered, how long it took, and what it answered, so the
-- owner can see the work and compare models on the same inputs.
CREATE TABLE task_runs (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind              text NOT NULL CHECK (btrim(kind) <> ''),
    subject_id        uuid,
    base_url          text NOT NULL DEFAULT '',
    model             text NOT NULL DEFAULT '',
    prompt_id         uuid REFERENCES agent_prompts (id) ON DELETE SET NULL,
    prompt_version    integer,
    input_hash        text NOT NULL DEFAULT '',
    output            jsonb,
    prompt_tokens     integer NOT NULL DEFAULT 0,
    completion_tokens integer NOT NULL DEFAULT 0,
    started_at        timestamptz NOT NULL,
    duration_ms       integer NOT NULL CHECK (duration_ms >= 0),
    outcome           text NOT NULL CHECK (outcome IN ('succeeded', 'failed', 'invalid')),
    error             text NOT NULL DEFAULT ''
);

CREATE INDEX task_runs_started ON task_runs (started_at DESC);
CREATE INDEX task_runs_kind_started ON task_runs (kind, started_at DESC);

-- +goose Down
DROP TABLE task_runs;
