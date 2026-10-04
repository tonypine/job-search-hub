package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// JobSourceHimalayas marks jobs gathered from the Himalayas job feed.
const JobSourceHimalayas = "himalayas"

// JobSourceRemoteOK marks jobs gathered from Remote OK's feed.
const JobSourceRemoteOK = "remoteok"

// JobSourceHackerNews marks jobs read from Hacker News' monthly "Who is
// hiring?" thread.
const JobSourceHackerNews = "hackernews"

// FeedSyncResult counts what one sync of a job feed changed.
type FeedSyncResult struct {
	Created  int `json:"created"`
	Closed   int `json:"closed"`
	Reopened int `json:"reopened"`
	Seen     int `json:"seen"`
	// ListedOnBoard counts new postings left out because the company's own
	// board lists them.
	ListedOnBoard int `json:"listed_on_board"`
}

// SyncFeedJobs stores the postings a feed returned, as seen at seenAt, then
// closes the feed's jobs whose expiry has passed. A feed returns only what a
// search matches, so a posting missing from it is not closed. A new posting
// is linked to the hub's company of the same name when there is one, and left
// out when that company's own board lists it under the same title. A known
// job given more text than an alert's snippet drops the reason it had none.
// Only lifecycle events are recorded as changes.
func (s *Store) SyncFeedJobs(ctx context.Context, actor Actor, source string, postings []JobPosting, seenAt time.Time) (FeedSyncResult, error) {
	result := FeedSyncResult{Seen: len(postings)}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		externalIDs := make([]string, 0, len(postings))
		for _, posting := range postings {
			externalIDs = append(externalIDs, posting.ExternalID)
		}
		rows, err := tx.Query(ctx, `SELECT external_id, id, closed_at FROM jobs WHERE source = $1 AND external_id = ANY($2) FOR UPDATE`, source, externalIDs)
		if err != nil {
			return err
		}
		type knownJob struct {
			id       uuid.UUID
			closedAt *time.Time
		}
		known := map[string]knownJob{}
		for rows.Next() {
			var externalID string
			var job knownJob
			if err := rows.Scan(&externalID, &job.id, &job.closedAt); err != nil {
				return err
			}
			known[externalID] = job
		}
		if err := rows.Err(); err != nil {
			return err
		}

		boardJobKeys, err := listOpenBoardJobKeys(ctx, tx)
		if err != nil {
			return err
		}
		for _, posting := range postings {
			existing, isKnown := known[posting.ExternalID]
			if !isKnown && boardJobKeys[NormalizeCompanyName(posting.CompanyName)+"/"+getTitleKey(posting.Title)] {
				result.ListedOnBoard++
				continue
			}
			if !isKnown {
				job, err := scanJob(tx.QueryRow(ctx, `
					INSERT INTO jobs (company_id, source, external_id, company_name, title, location, workplace_type, url, description, raw,
						first_seen_at, last_seen_at, expires_at, pay, employment_type, department, other_locations, published_at)
					VALUES ((SELECT id FROM companies WHERE lower(name) = lower($3) ORDER BY created_at LIMIT 1),
						$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10, $11, $12, $13, $14, $15, $16)
					RETURNING `+jobColumns,
					append([]any{source, posting.ExternalID, posting.CompanyName, posting.Title, posting.Location, posting.WorkplaceType,
						posting.URL, posting.Description, posting.Raw, seenAt, posting.ExpiresAt}, posting.boardFactsArguments()...)...))
				if err != nil {
					return err
				}
				result.Created++
				if err := insertChange(ctx, tx, actor, change{
					entityType: "job", entityID: job.ID, operation: "create",
					after: map[string]string{"title": job.Title, "url": job.URL}, sourceURL: job.URL,
				}); err != nil {
					return err
				}
				continue
			}

			if _, err := tx.Exec(ctx, `
				UPDATE jobs SET `+buildPollAssignment(JobFieldCompanyName, "$2")+`, `+buildPollAssignment(JobFieldTitle, "$3")+`,
					`+buildPollAssignment(JobFieldLocation, "$4")+`, `+buildPollAssignment(JobFieldWorkplaceType, "$5")+`,
					url = $6, description = $7, raw = $8, last_seen_at = $9, expires_at = $10, closed_at = NULL,
					`+buildPollAssignment(JobFieldPay, "$11")+`, `+buildPollAssignment(JobFieldEmploymentType, "$12")+`,
					department = $13, other_locations = $14, published_at = $15,
					text_missing_reason = CASE WHEN octet_length(btrim($7)) >= $16 THEN '' ELSE text_missing_reason END
				WHERE id = $1`,
				append(append([]any{existing.id, posting.CompanyName, posting.Title, posting.Location, posting.WorkplaceType, posting.URL,
					posting.Description, posting.Raw, seenAt, posting.ExpiresAt}, posting.boardFactsArguments()...), AlertSnippetLength)...); err != nil {
				return err
			}
			if existing.closedAt != nil {
				result.Reopened++
				if err := insertChange(ctx, tx, actor, change{entityType: "job", entityID: existing.id, operation: "reopen", sourceURL: posting.URL}); err != nil {
					return err
				}
			}
		}

		closedRows, err := tx.Query(ctx, `
			UPDATE jobs SET closed_at = $2
			WHERE source = $1 AND closed_at IS NULL AND expires_at < $2
			RETURNING id`, source, seenAt)
		if err != nil {
			return err
		}
		closedIDs, err := pgx.CollectRows(closedRows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		result.Closed = len(closedIDs)
		for _, closedID := range closedIDs {
			if err := insertChange(ctx, tx, actor, change{entityType: "job", entityID: closedID, operation: "close"}); err != nil {
				return err
			}
		}
		_, err = tieJobsToCompanies(ctx, tx)
		return err
	})
	return result, err
}

// listOpenBoardJobKeys returns the keys of the open jobs from companies' own
// boards: their company's normalized name and their title's key, joined by "/".
func listOpenBoardJobKeys(ctx context.Context, tx pgx.Tx) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT coalesce(companies.name, jobs.company_name), jobs.title
		FROM jobs LEFT JOIN companies ON companies.id = jobs.company_id
		WHERE jobs.job_board_id IS NOT NULL AND jobs.closed_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	boardJobKeys := map[string]bool{}
	for rows.Next() {
		var companyName, title string
		if err := rows.Scan(&companyName, &title); err != nil {
			return nil, err
		}
		boardJobKeys[NormalizeCompanyName(companyName)+"/"+getTitleKey(title)] = true
	}
	return boardJobKeys, rows.Err()
}
