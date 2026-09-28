-- +goose Up
-- The companies the owner follows on LinkedIn, from Company Follows.csv:
-- interest already shown, which the hub turns into watch-list suggestions.
CREATE TABLE linkedin_company_follows (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization text NOT NULL CHECK (organization <> ''),
    followed_at  timestamptz,
    UNIQUE (organization)
);

-- +goose Down
DROP TABLE linkedin_company_follows;
