-- +goose Up
-- The owner's LinkedIn profile as their export gives it: headline, summary,
-- positions, skills, education, languages, projects, courses and job-seeker
-- preferences. Address, birth date and phone are never kept.
CREATE TABLE linkedin_profile (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    singleton  boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton),
    snapshot   jsonb NOT NULL DEFAULT '{}',
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE linkedin_profile;
