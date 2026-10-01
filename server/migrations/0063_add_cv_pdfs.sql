-- +goose Up
-- A CV's PDF as the owner printed it on the Mac, kept with the CV.
ALTER TABLE cvs ADD COLUMN pdf bytea;

-- +goose Down
ALTER TABLE cvs DROP COLUMN pdf;
