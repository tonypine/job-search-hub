-- +goose Up
CREATE TABLE pipeline_phases (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL CHECK (name <> ''),
    position   integer NOT NULL,
    is_closed  boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX pipeline_phases_name ON pipeline_phases (lower(name));

INSERT INTO pipeline_phases (name, position, is_closed) VALUES
    ('Saved', 1, false),
    ('Applied', 2, false),
    ('In contact', 3, false),
    ('Interviewing', 4, false),
    ('Offer', 5, false),
    ('Closed', 6, true);

CREATE TABLE applications (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id           uuid REFERENCES jobs (id),
    company_id       uuid REFERENCES companies (id),
    phase_id         uuid NOT NULL REFERENCES pipeline_phases (id),
    closed_reason    text NOT NULL DEFAULT '',
    notes            text NOT NULL DEFAULT '',
    phase_entered_at timestamptz NOT NULL DEFAULT now(),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (job_id IS NOT NULL OR company_id IS NOT NULL)
);

CREATE UNIQUE INDEX applications_job ON applications (job_id) WHERE job_id IS NOT NULL;
CREATE INDEX applications_phase ON applications (phase_id);

-- +goose Down
DROP TABLE applications;
DROP TABLE pipeline_phases;
