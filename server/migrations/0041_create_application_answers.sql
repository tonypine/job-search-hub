-- +goose Up
-- The owner's answers to the questions application forms ask, for the agents
-- helping with an application: the same facts every time. question_key is
-- the question normalized, so an import never adds one twice.
CREATE TABLE application_answers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    question     text NOT NULL CHECK (btrim(question) <> ''),
    question_key text NOT NULL UNIQUE,
    answer       text NOT NULL DEFAULT '',
    source       text NOT NULL CHECK (source IN ('owner', 'linkedin')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE application_answers;
