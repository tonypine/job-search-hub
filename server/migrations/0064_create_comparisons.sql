-- +goose Up
-- A comparison runs one task kind on the same jobs through two or more
-- stacks (a model on a provider, Claude through the CLI, or answers imported
-- from elsewhere) so the owner can judge the answers side by side.
CREATE TABLE comparisons (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_kind  text NOT NULL,
    title      text NOT NULL CHECK (btrim(title) <> ''),
    status     text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'done')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE comparison_stacks (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    comparison_id uuid NOT NULL REFERENCES comparisons (id) ON DELETE CASCADE,
    position      integer NOT NULL,
    label         text NOT NULL CHECK (btrim(label) <> ''),
    source        text NOT NULL CHECK (source IN ('imported', 'route', 'claude')),
    provider_id   uuid REFERENCES model_providers (id) ON DELETE SET NULL,
    model         text NOT NULL DEFAULT '',
    UNIQUE (comparison_id, position)
);
CREATE TABLE comparison_jobs (
    comparison_id uuid NOT NULL REFERENCES comparisons (id) ON DELETE CASCADE,
    job_id        uuid NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    position      integer NOT NULL,
    PRIMARY KEY (comparison_id, job_id)
);
CREATE TABLE comparison_answers (
    stack_id uuid NOT NULL REFERENCES comparison_stacks (id) ON DELETE CASCADE,
    job_id   uuid NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    answer   jsonb,
    error    text NOT NULL DEFAULT '',
    PRIMARY KEY (stack_id, job_id)
);
CREATE TABLE comparison_verdicts (
    stack_id   uuid NOT NULL REFERENCES comparison_stacks (id) ON DELETE CASCADE,
    job_id     uuid NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    field      text NOT NULL,
    verdict    text NOT NULL CHECK (verdict IN ('right', 'wrong')),
    judged_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (stack_id, job_id, field)
);

-- +goose Down
DROP TABLE comparison_verdicts;
DROP TABLE comparison_answers;
DROP TABLE comparison_jobs;
DROP TABLE comparison_stacks;
DROP TABLE comparisons;
