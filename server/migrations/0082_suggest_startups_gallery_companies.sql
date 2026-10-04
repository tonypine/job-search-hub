-- +goose Up
-- A company startups.gallery lists as remote: only its name, site and
-- careers link, the board that link names, and the board kept for it once
-- its titles name a role.
CREATE TABLE gallery_companies (
    slug         text PRIMARY KEY,
    name         text NOT NULL,
    website      text NOT NULL DEFAULT '',
    careers_url  text NOT NULL DEFAULT '',
    provider     text NOT NULL DEFAULT '',
    board_token  text NOT NULL DEFAULT '',
    found_at     timestamptz NOT NULL DEFAULT now(),
    page_read_at timestamptz,
    checked_at   timestamptz,
    job_board_id uuid REFERENCES job_boards (id) ON DELETE SET NULL
);

-- Each weekly read of startups.gallery's remote list.
CREATE TABLE gallery_list_reads (
    read_at timestamptz NOT NULL DEFAULT now(),
    listed  integer NOT NULL
);

-- +goose Down
DROP TABLE gallery_list_reads;
DROP TABLE gallery_companies;
