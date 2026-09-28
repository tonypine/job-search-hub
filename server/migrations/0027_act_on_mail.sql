-- +goose Up
-- What the hub did about a message, once: acted_at is set when it acted or
-- decided there was nothing to do.
ALTER TABLE mail_messages
    ADD COLUMN acted_at timestamptz,
    ADD COLUMN action   text NOT NULL DEFAULT '';

CREATE INDEX mail_messages_awaiting_action ON mail_messages (sent_at) WHERE acted_at IS NULL;

-- When a person at the company first wrote back: the contact the search is
-- measured by.
ALTER TABLE applications ADD COLUMN contacted_at timestamptz;

-- +goose Down
ALTER TABLE applications DROP COLUMN contacted_at;
DROP INDEX mail_messages_awaiting_action;
ALTER TABLE mail_messages DROP COLUMN action, DROP COLUMN acted_at;
