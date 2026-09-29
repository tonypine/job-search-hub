-- +goose Up
-- Eightfold career sites, such as a large employer's <tenant>.eightfold.ai.
ALTER TABLE job_boards DROP CONSTRAINT job_boards_provider_check;
ALTER TABLE job_boards ADD CONSTRAINT job_boards_provider_check
    CHECK (provider IN ('greenhouse', 'lever', 'ashby', 'workable', 'recruitee', 'personio', 'smartrecruiters', 'eightfold', 'other'));

-- +goose Down
UPDATE job_boards SET provider = 'other', verified_at = NULL WHERE provider = 'eightfold';
ALTER TABLE job_boards DROP CONSTRAINT job_boards_provider_check;
ALTER TABLE job_boards ADD CONSTRAINT job_boards_provider_check
    CHECK (provider IN ('greenhouse', 'lever', 'ashby', 'workable', 'recruitee', 'personio', 'smartrecruiters', 'other'));
