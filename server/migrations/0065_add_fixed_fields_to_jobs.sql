-- +goose Up
-- The fields a fix corrected; board, feed and careers-page polls leave them
-- as fixed and refresh the rest.
ALTER TABLE jobs ADD COLUMN fixed_fields text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE jobs DROP COLUMN fixed_fields;
