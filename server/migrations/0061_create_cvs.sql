-- +goose Up
-- CVs as JSON Resume content: the owner's base CV, and the drafts tailored
-- for jobs. citations holds a tailored draft's source for each bullet.
CREATE TABLE cvs (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind       text NOT NULL CHECK (kind IN ('base', 'tailored')),
    job_id     uuid REFERENCES jobs (id) ON DELETE CASCADE,
    content    jsonb NOT NULL,
    citations  jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'base') = (job_id IS NULL))
);
CREATE UNIQUE INDEX cvs_one_base ON cvs (kind) WHERE kind = 'base';
CREATE INDEX cvs_job ON cvs (job_id);

-- +goose Down
DROP TABLE cvs;
