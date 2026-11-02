-- +goose Up
-- A job's pay keeps its ranges as a list, empty when it publishes none: a
-- fix that sent a summary alone stored them as null, which the apps can't read.
UPDATE jobs SET pay = jsonb_set(pay, '{ranges}', '[]'::jsonb)
WHERE jsonb_typeof(pay) = 'object' AND (NOT pay ? 'ranges' OR jsonb_typeof(pay->'ranges') = 'null');

-- +goose Down
-- Empty ranges are what the code writes now, so there is nothing to undo.
SELECT 1;
