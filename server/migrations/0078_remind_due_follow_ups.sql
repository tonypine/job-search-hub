-- +goose Up
-- When a card's follow-up falls due the hub tells the owner once; this is the
-- due time it last told them about, so the next due time is told again.
ALTER TABLE applications ADD COLUMN follow_up_reminded_at timestamptz;

-- +goose Down
ALTER TABLE applications DROP COLUMN follow_up_reminded_at;
