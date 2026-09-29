-- +goose Up
-- People the owner knows who don't work at a company but can open doors
-- there, like a former colleague who interviewed at it. Minimal fields, for
-- the owner's use only, deletable.
CREATE TABLE network_contacts (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name              text NOT NULL CHECK (btrim(name) <> ''),
    how_known         text NOT NULL DEFAULT '',
    preferred_channel text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX network_contacts_name ON network_contacts (lower(btrim(name)));

-- How a contact can help at one company, e.g. "interviewed there".
CREATE TABLE network_contact_companies (
    contact_id uuid NOT NULL REFERENCES network_contacts (id) ON DELETE CASCADE,
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    note       text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (contact_id, company_id)
);

CREATE INDEX network_contact_companies_company ON network_contact_companies (company_id);

-- +goose Down
DROP TABLE network_contact_companies;
DROP TABLE network_contacts;
