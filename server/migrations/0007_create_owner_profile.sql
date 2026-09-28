-- +goose Up
-- One row: the candidate the hub works for.
CREATE TABLE owner_profile (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    singleton  boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton),
    body       text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO owner_profile DEFAULT VALUES;

-- +goose Down
DROP TABLE owner_profile;
