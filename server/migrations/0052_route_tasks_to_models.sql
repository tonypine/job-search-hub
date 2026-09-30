-- +goose Up
-- The model servers the hub can call, local or hosted, and which one each
-- kind of task runs on, with a fallback. A provider's key is never served
-- back; a provider that can't enforce a schema has its answers validated.
CREATE TABLE model_providers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    base_url        text NOT NULL CHECK (base_url ~ '^https?://'),
    api_key         text NOT NULL DEFAULT '',
    enforces_schema boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE task_routes (
    kind                 text PRIMARY KEY CHECK (btrim(kind) <> ''),
    provider_id          uuid NOT NULL REFERENCES model_providers (id),
    model                text NOT NULL CHECK (btrim(model) <> ''),
    fallback_provider_id uuid REFERENCES model_providers (id) ON DELETE SET NULL,
    fallback_model       text NOT NULL DEFAULT '',
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE task_routes;
DROP TABLE model_providers;
