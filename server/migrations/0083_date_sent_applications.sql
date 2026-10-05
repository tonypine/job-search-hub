-- +goose Up
-- When the application went out: set once, the first time the card reached
-- Applied or a later open phase, so a card closed after it was sent still
-- counts as sent and one dropped while Saved doesn't.
ALTER TABLE applications ADD COLUMN applied_at timestamptz;

-- Existing cards take it from the change log: the first move into a phase
-- that counts as sent, or a card added already sent, as the LinkedIn import
-- adds an application in Applied, or in the closed phase when it went out
-- long ago, entering it on the application's date. A card sitting in a sent
-- phase went out by the time it entered it. A phase counts as sent from
-- Applied on, or from the second open phase when none is named so, as the
-- apps' contact tally reads the board.
WITH sent_phases AS (
    SELECT id FROM pipeline_phases
    WHERE NOT is_closed AND position >= COALESCE(
        (SELECT min(position) FROM pipeline_phases WHERE NOT is_closed AND lower(name) = 'applied'),
        (SELECT position FROM pipeline_phases WHERE NOT is_closed ORDER BY position OFFSET 1 LIMIT 1))
),
sent_entries AS (
    SELECT changes.entity_id AS application_id,
           min(CASE WHEN changes.operation = 'create' THEN (changes.after->>'phase_entered_at')::timestamptz ELSE changes.created_at END) AS applied_at
    FROM changes
    WHERE changes.entity_type = 'application' AND (
        (changes.operation = 'move' AND changes.after->>'phase_id' IN (SELECT id::text FROM sent_phases))
        OR (changes.operation = 'create' AND changes.after->>'phase_id' IN (
            SELECT id::text FROM sent_phases UNION SELECT id::text FROM pipeline_phases WHERE is_closed)))
    GROUP BY changes.entity_id
)
UPDATE applications SET applied_at = LEAST(
    (SELECT sent_entries.applied_at FROM sent_entries WHERE sent_entries.application_id = applications.id),
    CASE WHEN applications.phase_id IN (SELECT id FROM sent_phases) THEN applications.phase_entered_at END);

-- +goose Down
ALTER TABLE applications DROP COLUMN applied_at;
