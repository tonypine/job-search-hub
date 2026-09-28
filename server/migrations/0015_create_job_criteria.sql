-- +goose Up
-- The one record of what makes a job worth the owner's time. It starts
-- empty; the owner fills it through the hub's tools.
CREATE TABLE job_criteria (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    singleton  boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton),
    body       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(body) = 'object'),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO job_criteria DEFAULT VALUES;

-- +goose Down
DROP TABLE job_criteria;
