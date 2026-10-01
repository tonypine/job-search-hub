package store

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// JobSourceCareersPage marks jobs an agent read off a company's careers page
// that no board the hub reads serves.
const JobSourceCareersPage = "careers_page"

// CareersPageSyncResult counts what one reading of a careers page changed.
type CareersPageSyncResult struct {
	Created       int `json:"created"`
	Closed        int `json:"closed"`
	Reopened      int `json:"reopened"`
	Seen          int `json:"seen"`
	AlreadyListed int `json:"already_listed"`
}

// SyncCareersPageJobs stores every open role the company's careers page
// lists, each known by its link. A role from an earlier reading that the page
// no longer lists closes. A role the hub already lists for the company from
// another source, under the same title, is left out.
func (s *Store) SyncCareersPageJobs(ctx context.Context, actor Actor, companyID uuid.UUID, postings []JobPosting, seenAt time.Time) (CareersPageSyncResult, error) {
	result := CareersPageSyncResult{Seen: len(postings)}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var companyName string
		if err := tx.QueryRow(ctx, `SELECT name FROM companies WHERE id = $1`, companyID).Scan(&companyName); err != nil {
			if err == pgx.ErrNoRows {
				return ErrCompanyNotFound
			}
			return err
		}
		listedRows, err := tx.Query(ctx, `
			SELECT title FROM jobs WHERE company_id = $1 AND source <> $2 AND closed_at IS NULL`, companyID, JobSourceCareersPage)
		if err != nil {
			return err
		}
		listedTitles, err := pgx.CollectRows(listedRows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		listed := map[string]bool{}
		for _, title := range listedTitles {
			listed[normalizeTitle(title)] = true
		}

		kept := map[string]bool{}
		for _, posting := range postings {
			if listed[normalizeTitle(posting.Title)] {
				result.AlreadyListed++
				continue
			}
			kept[posting.ExternalID] = true
			var jobID uuid.UUID
			var wasClosed, created bool
			err := tx.QueryRow(ctx, `
				WITH existing AS (
					SELECT id, closed_at IS NOT NULL AS was_closed FROM jobs WHERE source = $1 AND external_id = $2 AND job_board_id IS NULL
				), updated AS (
					UPDATE jobs SET `+buildPollAssignment(JobFieldTitle, "$4")+`, `+buildPollAssignment(JobFieldLocation, "$5")+`,
						`+buildPollAssignment(JobFieldWorkplaceType, "$6")+`, url = $2, description = $7, `+buildPollAssignment(JobFieldEmploymentType, "$8")+`,
						`+buildPollAssignment(JobFieldCompanyID, "$3")+`, `+buildPollAssignment(JobFieldCompanyName, "$9")+`, last_seen_at = $10, closed_at = NULL
					WHERE id = (SELECT id FROM existing) RETURNING id
				), inserted AS (
					INSERT INTO jobs (company_id, source, external_id, company_name, title, location, workplace_type, url, description,
						employment_type, first_seen_at, last_seen_at)
					SELECT $3, $1, $2, $9, $4, $5, $6, $2, $7, $8, $10, $10 WHERE NOT EXISTS (SELECT 1 FROM existing)
					RETURNING id
				)
				SELECT id, coalesce((SELECT was_closed FROM existing), false), NOT EXISTS (SELECT 1 FROM existing)
				FROM (SELECT id FROM updated UNION ALL SELECT id FROM inserted) AS written`,
				JobSourceCareersPage, posting.ExternalID, companyID, posting.Title, posting.Location, posting.WorkplaceType,
				posting.Description, posting.EmploymentType, companyName, seenAt).Scan(&jobID, &wasClosed, &created)
			if err != nil {
				return err
			}
			switch {
			case created:
				result.Created++
				err = insertChange(ctx, tx, actor, change{
					entityType: "job", entityID: jobID, operation: "create",
					after: map[string]string{"title": posting.Title, "url": posting.ExternalID}, sourceURL: posting.ExternalID,
				})
			case wasClosed:
				result.Reopened++
				err = insertChange(ctx, tx, actor, change{entityType: "job", entityID: jobID, operation: "reopen", sourceURL: posting.ExternalID})
			}
			if err != nil {
				return err
			}
		}

		openRows, err := tx.Query(ctx, `
			SELECT id, external_id FROM jobs WHERE company_id = $1 AND source = $2 AND closed_at IS NULL`, companyID, JobSourceCareersPage)
		if err != nil {
			return err
		}
		type openJob struct {
			ID         uuid.UUID
			ExternalID string
		}
		openJobs, err := pgx.CollectRows(openRows, pgx.RowToStructByPos[openJob])
		if err != nil {
			return err
		}
		for _, job := range openJobs {
			if kept[job.ExternalID] {
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE jobs SET closed_at = $2 WHERE id = $1`, job.ID, seenAt); err != nil {
				return err
			}
			result.Closed++
			if err := insertChange(ctx, tx, actor, change{entityType: "job", entityID: job.ID, operation: "close"}); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

// normalizeTitle is a title without case, spaces or punctuation, so two
// listings of one role compare equal.
func normalizeTitle(title string) string {
	var normalized strings.Builder
	for _, character := range strings.ToLower(title) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	return normalized.String()
}
