-- +goose Up
-- What happened that the owner should hear about, each about a job, a
-- company, both, or nothing in particular. seen_at is null until the owner
-- sees it.
CREATE TABLE updates (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind       text NOT NULL CHECK (kind <> ''),
    title      text NOT NULL CHECK (title <> ''),
    body       text NOT NULL DEFAULT '',
    job_id     uuid REFERENCES jobs (id) ON DELETE CASCADE,
    company_id uuid REFERENCES companies (id) ON DELETE CASCADE,
    source_url text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    seen_at    timestamptz
);

CREATE INDEX updates_newest ON updates (created_at DESC);
CREATE INDEX updates_unseen_job ON updates (job_id) WHERE seen_at IS NULL;
CREATE INDEX updates_unseen_company ON updates (company_id) WHERE seen_at IS NULL;

-- +goose Down
DROP TABLE updates;
