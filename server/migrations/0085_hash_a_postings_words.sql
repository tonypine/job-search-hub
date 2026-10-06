-- +goose Up
-- A posting's words as its facts' hash reads them: without Markdown's
-- heading, list and bold markers, and without whitespace, so a description
-- that changes only in its markup, as when a board's HTML is kept as Markdown
-- instead of plain text, keeps the facts already read from it.
-- +goose StatementBegin
CREATE FUNCTION posting_words(description text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
AS $$
    SELECT regexp_replace(
        translate(
            replace(regexp_replace(description, '^[[:space:]]*((#{1,6}|[-*+]|[0-9]+\.)[[:space:]]+)+', '', 'gn'), '**', ''),
            E'\u0085                 　', ''),
        '[[:space:]]+', '', 'g')
$$;
-- +goose StatementEnd

-- Facts read from a job's current text keep matching it under the new hash.
UPDATE job_facts SET text_hash = sha256(convert_to(jobs.title || E'\n' || jobs.location || E'\n' || posting_words(jobs.description), 'UTF8'))
FROM jobs
WHERE jobs.id = job_facts.job_id
  AND job_facts.text_hash = sha256(convert_to(jobs.title || E'\n' || jobs.location || E'\n' || jobs.description, 'UTF8'));

-- +goose Down
UPDATE job_facts SET text_hash = sha256(convert_to(jobs.title || E'\n' || jobs.location || E'\n' || jobs.description, 'UTF8'))
FROM jobs
WHERE jobs.id = job_facts.job_id
  AND job_facts.text_hash = sha256(convert_to(jobs.title || E'\n' || jobs.location || E'\n' || posting_words(jobs.description), 'UTF8'));
DROP FUNCTION posting_words(text);
