-- +goose Up
CREATE TABLE agent_prompts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind       text NOT NULL CHECK (kind IN ('company_triage')),
    version    integer NOT NULL CHECK (version > 0),
    body       text NOT NULL CHECK (body <> ''),
    note       text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, version)
);

-- +goose Down
DROP TABLE agent_prompts;
