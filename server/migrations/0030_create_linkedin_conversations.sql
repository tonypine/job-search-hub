-- +goose Up
-- The owner's LinkedIn conversations, from the messages.csv of their own
-- data export. A re-import replaces a conversation's messages, since the
-- export gives messages no id.
CREATE TABLE linkedin_conversations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    linkedin_id     text NOT NULL UNIQUE CHECK (linkedin_id <> ''),
    title           text NOT NULL DEFAULT '',
    -- Who wrote the first message, whether it was the owner, and whether
    -- the owner ever wrote.
    started_by_url   text NOT NULL DEFAULT '',
    started_by_owner boolean NOT NULL DEFAULT false,
    owner_wrote      boolean NOT NULL DEFAULT false,
    message_count   integer NOT NULL DEFAULT 0,
    first_message_at timestamptz,
    last_message_at  timestamptz,
    imported_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE linkedin_messages (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id        uuid NOT NULL REFERENCES linkedin_conversations (id) ON DELETE CASCADE,
    sender_name            text NOT NULL DEFAULT '',
    sender_profile_url     text NOT NULL DEFAULT '',
    recipient_profile_urls text[] NOT NULL DEFAULT '{}',
    sent_at                timestamptz NOT NULL,
    subject                text NOT NULL DEFAULT '',
    content                text NOT NULL DEFAULT '',
    folder                 text NOT NULL DEFAULT ''
);

CREATE INDEX linkedin_messages_conversation ON linkedin_messages (conversation_id, sent_at);

CREATE TABLE linkedin_invitations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    direction    text NOT NULL CHECK (direction IN ('incoming', 'outgoing')),
    from_name    text NOT NULL DEFAULT '',
    to_name      text NOT NULL DEFAULT '',
    inviter_url  text NOT NULL DEFAULT '',
    invitee_url  text NOT NULL DEFAULT '',
    sent_at      timestamptz,
    message      text NOT NULL DEFAULT '',
    UNIQUE (inviter_url, invitee_url)
);

-- A connection's history with the owner, from the conversations they were
-- in: how many messages, when first and last, and whether they wrote first.
ALTER TABLE connections
    ADD COLUMN conversation_count integer NOT NULL DEFAULT 0,
    ADD COLUMN message_count      integer NOT NULL DEFAULT 0,
    ADD COLUMN first_message_at   timestamptz,
    ADD COLUMN last_message_at    timestamptz,
    ADD COLUMN they_wrote_first   boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE connections DROP COLUMN they_wrote_first, DROP COLUMN last_message_at, DROP COLUMN first_message_at,
    DROP COLUMN message_count, DROP COLUMN conversation_count;
DROP TABLE linkedin_invitations;
DROP TABLE linkedin_messages;
DROP TABLE linkedin_conversations;
