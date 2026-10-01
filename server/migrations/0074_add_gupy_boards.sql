-- +goose Up
-- Gupy career pages, <company>.gupy.io, where most Brazilian employers post.
ALTER TABLE job_boards DROP CONSTRAINT job_boards_provider_check;
ALTER TABLE job_boards ADD CONSTRAINT job_boards_provider_check
    CHECK (provider IN ('greenhouse', 'lever', 'ashby', 'workable', 'recruitee', 'personio', 'smartrecruiters', 'eightfold', 'bamboohr',
                        'pinpoint', 'gupy', 'other'));

-- +goose Down
UPDATE job_boards SET provider = 'other', verified_at = NULL WHERE provider = 'gupy';
ALTER TABLE job_boards DROP CONSTRAINT job_boards_provider_check;
ALTER TABLE job_boards ADD CONSTRAINT job_boards_provider_check
    CHECK (provider IN ('greenhouse', 'lever', 'ashby', 'workable', 'recruitee', 'personio', 'smartrecruiters', 'eightfold', 'bamboohr',
                        'pinpoint', 'other'));
