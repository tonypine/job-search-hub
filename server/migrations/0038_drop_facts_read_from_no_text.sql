-- +goose Up
-- Facts read from a job without a description are the model's musings about
-- an empty posting; the reader now leaves such jobs alone.
DELETE FROM job_facts WHERE job_id IN (SELECT id FROM jobs WHERE btrim(description) = '');

-- +goose Down
-- The deleted facts held nothing worth restoring.
SELECT 1;
