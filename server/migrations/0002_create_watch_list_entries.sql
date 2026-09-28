-- +goose Up
CREATE TABLE watch_list_entries (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies (id),
    added_at   timestamptz NOT NULL DEFAULT now(),
    removed_at timestamptz
);

CREATE UNIQUE INDEX watch_list_entries_active_company ON watch_list_entries (company_id) WHERE removed_at IS NULL;

-- +goose Down
DROP TABLE watch_list_entries;
