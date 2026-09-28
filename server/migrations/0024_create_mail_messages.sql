-- +goose Up
-- The owner's mail as Gmail announces it: who sent what to whom, and when.
-- Bodies stay in Gmail and are read on demand.
CREATE TABLE mail_messages (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    gmail_message_id text NOT NULL UNIQUE CHECK (gmail_message_id <> ''),
    thread_id        text NOT NULL,
    direction        text NOT NULL CHECK (direction IN ('received', 'sent')),
    sender           text NOT NULL DEFAULT '',
    recipients       text NOT NULL DEFAULT '',
    subject          text NOT NULL DEFAULT '',
    sent_at          timestamptz NOT NULL,
    label_ids        text[] NOT NULL DEFAULT '{}',
    recorded_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX mail_messages_thread ON mail_messages (thread_id);
CREATE INDEX mail_messages_newest ON mail_messages (sent_at DESC);

-- Where reading Gmail's changes resumes, and when Gmail stops announcing
-- them unless the watch is renewed.
ALTER TABLE google_connection
    ADD COLUMN gmail_history_id       text NOT NULL DEFAULT '',
    ADD COLUMN gmail_watch_expires_at timestamptz;

-- +goose Down
ALTER TABLE google_connection DROP COLUMN gmail_watch_expires_at, DROP COLUMN gmail_history_id;
DROP TABLE mail_messages;
