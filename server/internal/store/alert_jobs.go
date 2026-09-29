package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Sources of the job alert emails the owner receives, and of the jobs read
// from them.
const (
	JobSourceIndeed    = "indeed"
	JobSourceLinkedIn  = "linkedin"
	JobSourceGlassdoor = "glassdoor"
)

// ListUnreadJobAlerts returns received mail classified as a job alert and
// sent since the given time whose jobs haven't been read, oldest first.
func (s *Store) ListUnreadJobAlerts(ctx context.Context, since time.Time, limit int) ([]MailMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+mailMessageColumns+` FROM mail_messages
		WHERE classification = $1 AND direction = $2 AND alert_jobs_read_at IS NULL AND sent_at >= $3
		ORDER BY sent_at LIMIT $4`, MailJobAlert, MailReceived, since, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailMessage, error) { return scanMailMessage(row) })
}

// MarkJobAlertRead records that an alert's jobs were read, and how many it
// listed.
func (s *Store) MarkJobAlertRead(ctx context.Context, messageID uuid.UUID, found int) error {
	_, err := s.pool.Exec(ctx, `UPDATE mail_messages SET alert_jobs_read_at = now(), alert_jobs_found = $2 WHERE id = $1`, messageID, found)
	return err
}

// SyncAlertJobs stores the postings an alert listed as a feed's, seen when
// the alert was sent. A posting already open in the hub from another source,
// by the same company under the same title, is left out: it returns how many.
func (s *Store) SyncAlertJobs(ctx context.Context, actor Actor, source string, postings []JobPosting, seenAt time.Time) (FeedSyncResult, int, error) {
	titles := make([]string, 0, len(postings))
	for _, posting := range postings {
		titles = append(titles, posting.Title)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT posting.title, jobs.company_name, coalesce(companies.name, '')
		FROM unnest($2::text[]) AS posting (title)
		JOIN jobs ON jobs.closed_at IS NULL AND jobs.source <> $1
			AND regexp_replace(lower(jobs.title), '[^[:alnum:]]+', '', 'g') = regexp_replace(lower(posting.title), '[^[:alnum:]]+', '', 'g')
		LEFT JOIN companies ON companies.id = jobs.company_id`, source, titles)
	if err != nil {
		return FeedSyncResult{}, 0, err
	}
	listed := map[[2]string]bool{}
	for rows.Next() {
		var title, companyName, hubCompanyName string
		if err := rows.Scan(&title, &companyName, &hubCompanyName); err != nil {
			return FeedSyncResult{}, 0, err
		}
		listed[[2]string{title, NormalizeCompanyName(companyName)}] = true
		listed[[2]string{title, NormalizeCompanyName(hubCompanyName)}] = true
	}
	if err := rows.Err(); err != nil {
		return FeedSyncResult{}, 0, err
	}
	fresh := make([]JobPosting, 0, len(postings))
	for _, posting := range postings {
		if !listed[[2]string{posting.Title, NormalizeCompanyName(posting.CompanyName)}] {
			fresh = append(fresh, posting)
		}
	}
	result, err := s.SyncFeedJobs(ctx, actor, source, fresh, seenAt)
	return result, len(postings) - len(fresh), err
}
