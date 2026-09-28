-- +goose Up
-- The owner's LinkedIn connections, from the Connections.csv of their own
-- data export; the hub never fetches linkedin.com. company_id ties one to a
-- hub company whose name matches where they work.
CREATE TABLE connections (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name   text NOT NULL DEFAULT '',
    last_name    text NOT NULL DEFAULT '',
    profile_url  text NOT NULL CHECK (profile_url <> ''),
    email        text NOT NULL DEFAULT '',
    company_name text NOT NULL DEFAULT '',
    position     text NOT NULL DEFAULT '',
    connected_on date,
    company_id   uuid REFERENCES companies (id) ON DELETE SET NULL,
    imported_at  timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX connections_profile_url ON connections (lower(profile_url));
CREATE INDEX connections_company ON connections (company_id);

-- +goose Down
DROP TABLE connections;
