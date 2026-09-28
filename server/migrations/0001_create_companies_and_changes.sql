-- +goose Up
CREATE TABLE companies (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 text NOT NULL,
    domain               text NOT NULL UNIQUE,
    website_url          text NOT NULL DEFAULT '',
    careers_url          text NOT NULL DEFAULT '',
    headquarters_country text NOT NULL DEFAULT '',
    employee_count_range text NOT NULL DEFAULT '',
    summary              text NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE changes (
    id           bigserial PRIMARY KEY,
    actor_kind   text NOT NULL CHECK (actor_kind IN ('owner', 'agent_run')),
    agent_run_id uuid,
    entity_type  text NOT NULL,
    entity_id    uuid NOT NULL,
    operation    text NOT NULL,
    before       jsonb,
    after        jsonb,
    source_url   text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK ((actor_kind = 'agent_run') = (agent_run_id IS NOT NULL))
);

CREATE INDEX changes_entity ON changes (entity_type, entity_id);
CREATE INDEX changes_agent_run ON changes (agent_run_id) WHERE agent_run_id IS NOT NULL;

-- +goose Down
DROP TABLE changes;
DROP TABLE companies;
