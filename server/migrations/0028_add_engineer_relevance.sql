-- +goose Up
-- An engineer on the team the candidate would join: a peer to ask about the
-- work, where directors and executives rarely answer.
ALTER TABLE people DROP CONSTRAINT people_relevance_check;
ALTER TABLE people ADD CONSTRAINT people_relevance_check
    CHECK (relevance IN ('hiring_manager', 'engineering_lead', 'engineer', 'recruiter', 'founder', 'other'));

-- +goose Down
UPDATE people SET relevance = 'other' WHERE relevance = 'engineer';
ALTER TABLE people DROP CONSTRAINT people_relevance_check;
ALTER TABLE people ADD CONSTRAINT people_relevance_check
    CHECK (relevance IN ('hiring_manager', 'engineering_lead', 'recruiter', 'founder', 'other'));
