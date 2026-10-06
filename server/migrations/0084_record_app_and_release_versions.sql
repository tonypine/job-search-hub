-- +goose Up
-- The version of the app each phone last called the hub with, from its
-- X-Hub-Client header, so the Mac can show which phone is behind.
ALTER TABLE devices ADD COLUMN app_version text;

-- The release that first migrated the database to each migration, so an
-- older server that finds the database ahead of it can name the release to
-- install. A dev build records nothing.
CREATE TABLE migration_releases (
    migration bigint PRIMARY KEY,
    release text NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE migration_releases;
ALTER TABLE devices DROP COLUMN app_version;
