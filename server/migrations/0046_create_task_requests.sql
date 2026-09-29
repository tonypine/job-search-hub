-- +goose Up
-- Work the owner asks the Mac to do from the phone. Agents run only on the
-- Mac, so the hub queues the request and the Mac app claims and runs it.
CREATE TABLE task_requests (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind        text NOT NULL CHECK (kind IN ('find_jobs', 'research_company')),
    company_id  uuid REFERENCES companies (id) ON DELETE CASCADE,
    input       text NOT NULL DEFAULT '',
    status      text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    result      text NOT NULL DEFAULT '',
    device_id   uuid REFERENCES devices (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    started_at  timestamptz,
    finished_at timestamptz
);

CREATE INDEX task_requests_queued ON task_requests (created_at) WHERE status = 'queued';

-- +goose Down
DROP TABLE task_requests;
