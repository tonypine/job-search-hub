-- +goose Up
-- Claude sessions Tony runs from the app, each about one company or one job.
-- Whether a session is running is not stored: the app's processes say so.
CREATE TABLE claude_sessions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        uuid REFERENCES companies (id) ON DELETE CASCADE,
    job_id            uuid REFERENCES jobs (id) ON DELETE CASCADE,
    claude_session_id uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    name              text NOT NULL CHECK (name <> ''),
    created_at        timestamptz NOT NULL DEFAULT now(),
    last_started_at   timestamptz,
    last_stopped_at   timestamptz,
    CHECK ((company_id IS NULL) <> (job_id IS NULL))
);

CREATE INDEX claude_sessions_company ON claude_sessions (company_id) WHERE company_id IS NOT NULL;
CREATE INDEX claude_sessions_job ON claude_sessions (job_id) WHERE job_id IS NOT NULL;

-- +goose Down
DROP TABLE claude_sessions;
