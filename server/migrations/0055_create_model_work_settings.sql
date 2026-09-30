-- +goose Up
-- Whether background model work is paused. One row. It starts paused, so a
-- hub that gets its own model runtime does no work until the owner resumes it.
CREATE TABLE model_work_settings (
    id         boolean PRIMARY KEY DEFAULT true CHECK (id),
    paused     boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO model_work_settings DEFAULT VALUES;

-- +goose Down
DROP TABLE model_work_settings;
