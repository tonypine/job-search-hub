-- +goose Up
-- The Firebase Cloud Messaging token each phone registers, so the hub can push
-- its updates. FCM rotates these, and the phone registers the new one.
ALTER TABLE devices ADD COLUMN push_token text UNIQUE;

-- +goose Down
ALTER TABLE devices DROP COLUMN push_token;
