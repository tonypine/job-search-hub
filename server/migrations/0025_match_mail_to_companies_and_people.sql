-- +goose Up
-- A person's work email, from a public page or their own mail; never guessed.
ALTER TABLE people ADD COLUMN email text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX people_email ON people (lower(email)) WHERE email <> '';

-- What a message is about, and which rule said so; an empty matched_by is a
-- message not matched yet.
ALTER TABLE mail_messages
    ADD COLUMN company_id uuid REFERENCES companies (id) ON DELETE SET NULL,
    ADD COLUMN person_id  uuid REFERENCES people (id) ON DELETE SET NULL,
    ADD COLUMN matched_by text NOT NULL DEFAULT ''
        CHECK (matched_by IN ('', 'person', 'domain', 'thread', 'applicant_tracking'));

CREATE INDEX mail_messages_company ON mail_messages (company_id);

-- +goose Down
DROP INDEX mail_messages_company;
ALTER TABLE mail_messages DROP COLUMN matched_by, DROP COLUMN person_id, DROP COLUMN company_id;
DROP INDEX people_email;
ALTER TABLE people DROP COLUMN email;
