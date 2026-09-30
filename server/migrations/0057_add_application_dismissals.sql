-- +goose Up
-- A card the owner decided against, as not a good fit, leaves the board
-- without moving phase. A card with a job follows its job's dismissal; only a
-- card for a company alone keeps its own.
ALTER TABLE applications
    ADD COLUMN dismissed_at     timestamptz,
    ADD COLUMN dismissal_reason text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE applications DROP COLUMN dismissal_reason, DROP COLUMN dismissed_at;
