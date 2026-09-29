-- +goose Up
-- The owner's experience as separate entries: roles held, cases of work within
-- them, skills, education, projects, preferences and other facts. Agents write
-- entries unconfirmed; only the owner confirms one, and only confirmed entries
-- speak for the owner in briefs and CVs. Months are YYYY or YYYY-MM, as CVs
-- give them.
CREATE TABLE profile_entries (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind          text NOT NULL CHECK (kind IN ('role', 'case', 'skill', 'education', 'project', 'preference', 'fact')),
    role_id       uuid REFERENCES profile_entries (id) ON DELETE SET NULL,
    title         text NOT NULL CHECK (btrim(title) <> ''),
    body          text NOT NULL DEFAULT '',
    organization  text NOT NULL DEFAULT '',
    start_month   text NOT NULL DEFAULT '' CHECK (start_month = '' OR start_month ~ '^[0-9]{4}(-(0[1-9]|1[0-2]))?$'),
    end_month     text NOT NULL DEFAULT '' CHECK (end_month = '' OR end_month ~ '^[0-9]{4}(-(0[1-9]|1[0-2]))?$'),
    skills        text[] NOT NULL DEFAULT '{}',
    outcome       text NOT NULL DEFAULT '',
    source        text NOT NULL CHECK (source IN ('cv', 'linkedin', 'interview', 'owner')),
    source_detail text NOT NULL DEFAULT '',
    confirmed_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (role_id IS NULL OR kind IN ('case', 'skill', 'project'))
);

CREATE INDEX profile_entries_role ON profile_entries (role_id);

-- +goose Down
DROP TABLE profile_entries;
