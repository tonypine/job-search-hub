-- +goose Up
-- Phones paired with the hub, each with its own token, stored only as a hash,
-- so losing one means revoking one device rather than the owner's token.
CREATE TABLE devices (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name         text NOT NULL CHECK (btrim(name) <> ''),
    token_hash   bytea NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz,
    revoked_at   timestamptz
);

-- +goose Down
DROP TABLE devices;
