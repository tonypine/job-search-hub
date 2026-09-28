-- +goose Up
CREATE TABLE job_boards (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id         uuid NOT NULL REFERENCES companies (id),
    provider           text NOT NULL CHECK (provider IN
                           ('greenhouse', 'lever', 'ashby', 'workable', 'recruitee', 'personio', 'smartrecruiters', 'other')),
    board_token        text NOT NULL,
    board_url          text NOT NULL DEFAULT '',
    verified_at        timestamptz,
    open_posting_count integer,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, board_token)
);

CREATE INDEX job_boards_company ON job_boards (company_id);

-- +goose Down
DROP TABLE job_boards;
