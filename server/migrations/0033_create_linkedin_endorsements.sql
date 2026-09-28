-- +goose Up
-- Who vouched for the owner, and whom the owner vouched for, from the
-- endorsement and recommendation files of their LinkedIn export.
CREATE TABLE linkedin_endorsements (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    direction   text NOT NULL CHECK (direction IN ('received', 'given')),
    skill       text NOT NULL,
    first_name  text NOT NULL DEFAULT '',
    last_name   text NOT NULL DEFAULT '',
    profile_url text NOT NULL DEFAULT '',
    endorsed_at timestamptz,
    status      text NOT NULL DEFAULT '',
    UNIQUE (direction, profile_url, skill)
);

CREATE TABLE linkedin_recommendations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    direction  text NOT NULL CHECK (direction IN ('received', 'given')),
    first_name text NOT NULL DEFAULT '',
    last_name  text NOT NULL DEFAULT '',
    company    text NOT NULL DEFAULT '',
    job_title  text NOT NULL DEFAULT '',
    text       text NOT NULL DEFAULT '',
    written_at timestamptz,
    status     text NOT NULL DEFAULT '',
    UNIQUE (direction, first_name, last_name, written_at)
);

-- What vouching joins a connection and the owner.
ALTER TABLE connections
    ADD COLUMN endorsed_owner_for text[] NOT NULL DEFAULT '{}',
    ADD COLUMN owner_endorsed     boolean NOT NULL DEFAULT false,
    ADD COLUMN recommended_owner  boolean NOT NULL DEFAULT false,
    ADD COLUMN owner_recommended  boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE connections DROP COLUMN owner_recommended, DROP COLUMN recommended_owner, DROP COLUMN owner_endorsed,
    DROP COLUMN endorsed_owner_for;
DROP TABLE linkedin_recommendations;
DROP TABLE linkedin_endorsements;
