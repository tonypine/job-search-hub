-- +goose Up
-- Whether each kind of run the hub starts by itself is paused, in place of
-- the one pause for all model work. Every kind starts as that pause was, so a
-- hub that was paused stays paused and a new hub starts with every kind
-- paused. A kind the owner only starts has no row: it can't be paused.
CREATE TABLE run_kind_settings (
    kind       text PRIMARY KEY,
    paused     boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO run_kind_settings (kind, paused)
SELECT kind, coalesce((SELECT paused FROM model_work_settings), true)
FROM unnest(ARRAY[
    'job_facts', 'job_brief', 'job_brief_full', 'job_cv', 'recruiter_screen', 'interview_prep',
    'mail_triage', 'linkedin_conversation', 'market_gaps', 'hiring_thread'
]) AS kind;
DROP TABLE model_work_settings;

-- +goose Down
-- The one pause holds while any kind is paused, as a new hub's does.
CREATE TABLE model_work_settings (
    id         boolean PRIMARY KEY DEFAULT true CHECK (id),
    paused     boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO model_work_settings (paused)
SELECT coalesce(bool_or(paused), true) FROM run_kind_settings;
DROP TABLE run_kind_settings;
