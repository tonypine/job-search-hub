-- +goose Up
-- BambooHR careers sites and Pinpoint boards, such as <name>.bamboohr.com.
ALTER TABLE job_boards DROP CONSTRAINT job_boards_provider_check;
ALTER TABLE job_boards ADD CONSTRAINT job_boards_provider_check
    CHECK (provider IN ('greenhouse', 'lever', 'ashby', 'workable', 'recruitee', 'personio', 'smartrecruiters', 'eightfold', 'bamboohr',
                        'pinpoint', 'other'));

-- +goose Down
UPDATE job_boards SET provider = 'other', verified_at = NULL WHERE provider IN ('bamboohr', 'pinpoint');
ALTER TABLE job_boards DROP CONSTRAINT job_boards_provider_check;
ALTER TABLE job_boards ADD CONSTRAINT job_boards_provider_check
    CHECK (provider IN ('greenhouse', 'lever', 'ashby', 'workable', 'recruitee', 'personio', 'smartrecruiters', 'eightfold', 'other'));
