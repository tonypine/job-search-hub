-- +goose Up
ALTER TABLE changes DROP CONSTRAINT changes_actor_kind_check;
ALTER TABLE changes ADD CONSTRAINT changes_actor_kind_check CHECK (actor_kind IN ('owner', 'agent_run', 'system'));

CREATE TABLE jobs (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     uuid REFERENCES companies (id),
    job_board_id   uuid REFERENCES job_boards (id),
    external_id    text,
    source         text NOT NULL CHECK (source IN ('job_board', 'manual')),
    title          text NOT NULL CHECK (title <> ''),
    location       text NOT NULL DEFAULT '',
    workplace_type text NOT NULL DEFAULT '',
    url            text NOT NULL CHECK (url <> ''),
    description    text NOT NULL DEFAULT '',
    raw            jsonb,
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now(),
    closed_at      timestamptz,
    UNIQUE (job_board_id, external_id),
    CHECK ((source = 'job_board') = (job_board_id IS NOT NULL AND external_id IS NOT NULL))
);

CREATE UNIQUE INDEX jobs_manual_url ON jobs (url) WHERE source = 'manual';
CREATE INDEX jobs_company ON jobs (company_id);
CREATE INDEX jobs_open_first_seen ON jobs (first_seen_at DESC) WHERE closed_at IS NULL;

-- +goose Down
DROP TABLE jobs;
ALTER TABLE changes DROP CONSTRAINT changes_actor_kind_check;
ALTER TABLE changes ADD CONSTRAINT changes_actor_kind_check CHECK (actor_kind IN ('owner', 'agent_run'));
