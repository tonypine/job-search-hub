-- +goose Up
-- The owner's Google sign-in. The refresh token never leaves the server;
-- needs_reconnect_since is set when Google refuses it, e.g. after the 7 days
-- a "Testing" app's tokens last.
CREATE TABLE google_connection (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    singleton             boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton),
    email                 text NOT NULL DEFAULT '',
    refresh_token         text NOT NULL CHECK (refresh_token <> ''),
    scopes                text[] NOT NULL,
    connected_at          timestamptz NOT NULL DEFAULT now(),
    needs_reconnect_since timestamptz,
    last_error            text NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE google_connection;
