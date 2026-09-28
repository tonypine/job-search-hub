-- +goose Up
CREATE TABLE people (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  uuid NOT NULL REFERENCES companies (id),
    name        text NOT NULL,
    role_title  text NOT NULL DEFAULT '',
    relevance   text NOT NULL CHECK (relevance IN ('hiring_manager', 'engineering_lead', 'recruiter', 'founder', 'other')),
    profile_url text NOT NULL DEFAULT '',
    source_url  text NOT NULL CHECK (source_url <> ''),
    notes       text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX people_company_name ON people (company_id, lower(name));

-- +goose Down
DROP TABLE people;
