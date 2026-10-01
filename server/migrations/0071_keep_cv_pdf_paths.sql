-- +goose Up
-- Where the CV's PDF was printed to on this machine, so it can be opened,
-- shown in Finder or attached to a form.
ALTER TABLE cvs ADD COLUMN pdf_path text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE cvs DROP COLUMN pdf_path;
