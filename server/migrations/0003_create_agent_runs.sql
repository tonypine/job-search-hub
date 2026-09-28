-- +goose Up
CREATE TABLE agent_runs (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind              text NOT NULL CHECK (kind IN ('company_triage')),
    input             text NOT NULL,
    status            text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed')),
    token_hash        bytea NOT NULL UNIQUE,
    token_expires_at  timestamptz NOT NULL,
    claude_session_id text NOT NULL DEFAULT '',
    cost_usd_estimate numeric,
    result            jsonb,
    error             text NOT NULL DEFAULT '',
    started_at        timestamptz NOT NULL DEFAULT now(),
    finished_at       timestamptz
);

ALTER TABLE changes ADD CONSTRAINT changes_agent_run_id_fkey FOREIGN KEY (agent_run_id) REFERENCES agent_runs (id);

-- +goose Down
ALTER TABLE changes DROP CONSTRAINT changes_agent_run_id_fkey;
DROP TABLE agent_runs;
