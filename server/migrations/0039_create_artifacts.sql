-- +goose Up
-- Files the owner uploads by hand as context for the agents: a resume, a
-- company's document, a page they saved. Each is linked to the owner (no
-- company) or to one company, and stored once however often it's uploaded.
CREATE TABLE artifacts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id   uuid REFERENCES companies (id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('resume', 'company_document', 'saved_page', 'other')),
    name         text NOT NULL CHECK (name <> ''),
    content_type text NOT NULL DEFAULT '',
    size         integer NOT NULL CHECK (size > 0),
    sha256       bytea NOT NULL UNIQUE,
    content      bytea NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX artifacts_company ON artifacts (company_id);

-- +goose Down
DROP TABLE artifacts;
