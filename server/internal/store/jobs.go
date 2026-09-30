package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	JobSourceJobBoard = "job_board"
	JobSourceManual   = "manual"
)

type Job struct {
	ID            uuid.UUID  `json:"id"`
	CompanyID     *uuid.UUID `json:"company_id,omitempty"`
	JobBoardID    *uuid.UUID `json:"job_board_id,omitempty"`
	ExternalID    *string    `json:"external_id,omitempty"`
	Source        string     `json:"source"`
	Title         string     `json:"title"`
	Location      string     `json:"location,omitempty"`
	WorkplaceType string     `json:"workplace_type,omitempty"`
	URL           string     `json:"url"`
	Description   string     `json:"description,omitempty"`
	BoardFacts
	FirstSeenAt time.Time  `json:"first_seen_at"`
	LastSeenAt  time.Time  `json:"last_seen_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	// DismissedAt is when the owner dismissed the job; dismissed jobs leave
	// the jobs list until restored.
	DismissedAt     *time.Time `json:"dismissed_at,omitempty"`
	DismissalReason string     `json:"dismissal_reason,omitempty"`
}

// BoardFacts are what a job board publishes about a posting beyond its text.
type BoardFacts struct {
	Pay            *Pay       `json:"pay,omitempty"`
	EmploymentType string     `json:"employment_type,omitempty"`
	Department     string     `json:"department,omitempty"`
	OtherLocations []string   `json:"other_locations,omitempty"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
}

// Pay is the pay a posting publishes: one range per region or tier, and the
// board's own summary when it gives one, such as "$230K • Offers Equity".
type Pay struct {
	Ranges  []PayRange `json:"ranges"`
	Summary string     `json:"summary,omitempty"`
}

// PayRange is one published range. Interval is year, month, week, day or
// hour, and empty when the board does not say.
type PayRange struct {
	Label    string  `json:"label,omitempty"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Currency string  `json:"currency"`
	Interval string  `json:"interval,omitempty"`
}

const jobColumns = `id, company_id, job_board_id, external_id, source, title, location, workplace_type, url, description,
	pay, employment_type, department, other_locations, published_at, first_seen_at, last_seen_at, closed_at, dismissed_at, dismissal_reason`

// prefixedJobColumns are the jobColumns qualified for queries that join
// companies, whose id would otherwise be ambiguous.
const prefixedJobColumns = `jobs.id, jobs.company_id, jobs.job_board_id, jobs.external_id, jobs.source, jobs.title, jobs.location, jobs.workplace_type, jobs.url, jobs.description,
	jobs.pay, jobs.employment_type, jobs.department, jobs.other_locations, jobs.published_at, jobs.first_seen_at, jobs.last_seen_at, jobs.closed_at,
	jobs.dismissed_at, jobs.dismissal_reason`

// scanJob reads the jobColumns, then any extra columns the query selects
// after them into extra.
func scanJob(row pgx.Row, extra ...any) (Job, error) {
	var job Job
	destinations := append([]any{&job.ID, &job.CompanyID, &job.JobBoardID, &job.ExternalID, &job.Source, &job.Title, &job.Location,
		&job.WorkplaceType, &job.URL, &job.Description, &job.Pay, &job.EmploymentType, &job.Department, &job.OtherLocations, &job.PublishedAt,
		&job.FirstSeenAt, &job.LastSeenAt, &job.ClosedAt, &job.DismissedAt, &job.DismissalReason}, extra...)
	err := row.Scan(destinations...)
	return job, err
}

// JobPosting is one open posting as a job board or a job feed lists it.
// CompanyName and ExpiresAt come from feeds, whose postings belong to no
// board of the hub's.
type JobPosting struct {
	ExternalID    string
	CompanyName   string
	ExpiresAt     *time.Time
	Title         string
	Location      string
	WorkplaceType string
	URL           string
	Description   string
	BoardFacts
	Raw json.RawMessage
}

// boardFactsArguments are the posting's facts as query arguments, in the
// order pay, employment_type, department, other_locations, published_at.
func (posting JobPosting) boardFactsArguments() []any {
	otherLocations := posting.OtherLocations
	if otherLocations == nil {
		otherLocations = []string{}
	}
	return []any{posting.Pay, posting.EmploymentType, posting.Department, otherLocations, posting.PublishedAt}
}

// BoardSyncResult counts what one sync of a board changed.
type BoardSyncResult struct {
	Created  int `json:"created"`
	Closed   int `json:"closed"`
	Reopened int `json:"reopened"`
	Seen     int `json:"seen"`
}

// SyncBoardJobs makes the board's jobs match its current postings, as seen at
// seenAt: new postings are created, known ones are refreshed, postings that
// disappeared are closed, and a closed one that returns is reopened. Only
// those lifecycle events are recorded as changes; refreshing a posting that
// is still open is not.
func (s *Store) SyncBoardJobs(ctx context.Context, actor Actor, board JobBoard, postings []JobPosting, seenAt time.Time) (BoardSyncResult, error) {
	result := BoardSyncResult{Seen: len(postings)}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT external_id, id, closed_at FROM jobs WHERE job_board_id = $1 FOR UPDATE`, board.ID)
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

		seenExternalIDs := make([]string, 0, len(postings))
		for _, posting := range postings {
			seenExternalIDs = append(seenExternalIDs, posting.ExternalID)
			existing, isKnown := known[posting.ExternalID]
			if !isKnown {
				job, err := scanJob(tx.QueryRow(ctx, `
					INSERT INTO jobs (company_id, job_board_id, external_id, source, title, location, workplace_type, url, description, raw, first_seen_at, last_seen_at,
						pay, employment_type, department, other_locations, published_at)
					VALUES ($1, $2, $3, 'job_board', $4, $5, $6, $7, $8, $9, $10, $10, $11, $12, $13, $14, $15)
					RETURNING `+jobColumns,
					append([]any{board.CompanyID, board.ID, posting.ExternalID, posting.Title, posting.Location, posting.WorkplaceType,
						posting.URL, posting.Description, posting.Raw, seenAt}, posting.boardFactsArguments()...)...))
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
				UPDATE jobs SET title = $2, location = $3, workplace_type = $4, url = $5, description = $6, raw = $7,
					last_seen_at = $8, closed_at = NULL,
					pay = $9, employment_type = $10, department = $11, other_locations = $12, published_at = $13
				WHERE id = $1`,
				append([]any{existing.id, posting.Title, posting.Location, posting.WorkplaceType, posting.URL, posting.Description, posting.Raw, seenAt},
					posting.boardFactsArguments()...)...); err != nil {
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
			WHERE job_board_id = $1 AND closed_at IS NULL AND NOT (external_id = ANY($3))
			RETURNING id`, board.ID, seenAt, seenExternalIDs)
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
		return nil
	})
	return result, err
}

type ManualJobInput struct {
	CompanyID   *uuid.UUID
	Title       string
	URL         string
	Location    string
	Description string
}

// AddManualJob stores a job added by hand, or returns the one already added
// with the same URL; created reports which.
func (s *Store) AddManualJob(ctx context.Context, actor Actor, input ManualJobInput) (Job, bool, error) {
	title := strings.TrimSpace(input.Title)
	jobURL := strings.TrimSpace(input.URL)
	if title == "" || !(strings.HasPrefix(jobURL, "https://") || strings.HasPrefix(jobURL, "http://")) {
		return Job{}, false, errors.New("a job needs a title and an http(s) URL")
	}

	var job Job
	created := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		inserted, err := scanJob(tx.QueryRow(ctx, `
			INSERT INTO jobs (company_id, source, title, url, location, description)
			VALUES ($1, 'manual', $2, $3, $4, $5)
			ON CONFLICT (url) WHERE source = 'manual' DO NOTHING
			RETURNING `+jobColumns, input.CompanyID, title, jobURL, input.Location, input.Description))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			job, err = scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE source = 'manual' AND url = $1`, jobURL))
			return err
		case isForeignKeyViolation(err):
			return ErrCompanyNotFound
		case err != nil:
			return err
		}
		job = inserted
		created = true
		return insertChange(ctx, tx, actor, change{
			entityType: "job", entityID: job.ID, operation: "create",
			after: map[string]string{"title": job.Title, "url": job.URL}, sourceURL: job.URL,
		})
	})
	if err != nil {
		return Job{}, false, err
	}
	return job, created, nil
}

// ErrJobCompanyFromBoard means the job came from a company's own board, which
// already says whose it is.
var ErrJobCompanyFromBoard = errors.New("a job from a company's board keeps that company")

// SetJobCompany ties a job added by hand or found in a feed to its company,
// and moves the job's application to that company too.
func (s *Store) SetJobCompany(ctx context.Context, actor Actor, jobID, companyID uuid.UUID) (Job, error) {
	var job Job
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = $1 FOR UPDATE`, jobID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJobNotFound
		}
		if err != nil {
			return err
		}
		if current.Source == JobSourceJobBoard {
			return ErrJobCompanyFromBoard
		}
		job, err = scanJob(tx.QueryRow(ctx, `UPDATE jobs SET company_id = $2 WHERE id = $1 RETURNING `+jobColumns, jobID, companyID))
		if isForeignKeyViolation(err) {
			return ErrCompanyNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE applications SET company_id = $2, updated_at = now() WHERE job_id = $1`, jobID, companyID); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "job", entityID: jobID, operation: "update",
			before: map[string]any{"company_id": current.CompanyID}, after: map[string]any{"company_id": companyID},
		})
	})
	return job, err
}

// UpsertBoardJob stores one posting of a stored board, as a board sync would,
// without closing the board's other jobs; created reports whether it was new.
func (s *Store) UpsertBoardJob(ctx context.Context, actor Actor, board JobBoard, posting JobPosting, seenAt time.Time) (Job, bool, error) {
	var job Job
	created := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		job, err = scanJob(tx.QueryRow(ctx, `
			INSERT INTO jobs (company_id, job_board_id, external_id, source, title, location, workplace_type, url, description, raw, first_seen_at, last_seen_at,
				pay, employment_type, department, other_locations, published_at)
			VALUES ($1, $2, $3, 'job_board', $4, $5, $6, $7, $8, $9, $10, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (job_board_id, external_id) DO UPDATE SET
				title = EXCLUDED.title, location = EXCLUDED.location, workplace_type = EXCLUDED.workplace_type, url = EXCLUDED.url,
				description = EXCLUDED.description, raw = EXCLUDED.raw, last_seen_at = EXCLUDED.last_seen_at, closed_at = NULL,
				pay = EXCLUDED.pay, employment_type = EXCLUDED.employment_type, department = EXCLUDED.department,
				other_locations = EXCLUDED.other_locations, published_at = EXCLUDED.published_at
			RETURNING `+jobColumns+`, (xmax = 0)`,
			append([]any{board.CompanyID, board.ID, posting.ExternalID, posting.Title, posting.Location, posting.WorkplaceType,
				posting.URL, posting.Description, posting.Raw, seenAt}, posting.boardFactsArguments()...)...), &created)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "job", entityID: job.ID, operation: "create",
			after: map[string]string{"title": job.Title, "url": job.URL}, sourceURL: job.URL,
		})
	})
	return job, created, err
}

const (
	JobStatusOpen   = "open"
	JobStatusClosed = "closed"
	// JobStatusAll is every job that isn't dismissed.
	JobStatusAll       = "all"
	JobStatusDismissed = "dismissed"

	defaultJobPageSize = 100
	maximumJobPageSize = 500
)

// JobFilter narrows the jobs list. Query matches the title, location or
// company name, case-insensitively.
type JobFilter struct {
	Query     string
	CompanyID *uuid.UUID
	Status    string
	Limit     int
	Offset    int
}

// JobListItem is one row of the jobs list: a job, its company's name, and
// the facts read from it, which the fit is judged from.
type JobListItem struct {
	Job           Job     `json:"job"`
	CompanyName   *string `json:"company_name,omitempty"`
	UnseenUpdates int     `json:"unseen_updates"`
	// Facts are the facts read from the job's text, flattened by key (see
	// FlattenJobFacts); absent until read.
	Facts json.RawMessage `json:"facts,omitempty"`
}

// ListJobs returns one page of jobs, newest first, with the total that match.
func (s *Store) ListJobs(ctx context.Context, filter JobFilter) ([]JobListItem, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > maximumJobPageSize {
		limit = defaultJobPageSize
	}
	status := filter.Status
	if status == "" {
		status = JobStatusOpen
	}
	if status != JobStatusOpen && status != JobStatusClosed && status != JobStatusAll && status != JobStatusDismissed {
		return nil, 0, errors.New("status must be open, closed, all or dismissed")
	}

	const matches = `
		FROM jobs LEFT JOIN companies ON companies.id = jobs.company_id
		WHERE ($1 = '' OR strpos(lower(jobs.title), $1) > 0 OR strpos(lower(jobs.location), $1) > 0
		       OR strpos(lower(COALESCE(companies.name, jobs.company_name)), $1) > 0)
		  AND ($2::uuid IS NULL OR jobs.company_id = $2)
		  AND CASE WHEN $3 = 'dismissed' THEN jobs.dismissed_at IS NOT NULL
		           ELSE jobs.dismissed_at IS NULL AND ($3 = 'all' OR ($3 = 'open') = (jobs.closed_at IS NULL)) END`
	query := strings.ToLower(strings.TrimSpace(filter.Query))

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) `+matches, query, filter.CompanyID, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+prefixedJobColumns+`, COALESCE(companies.name, NULLIF(jobs.company_name, '')), `+jobUnseenUpdates+`,
		       (SELECT facts FROM job_facts WHERE job_facts.job_id = jobs.id)
		`+matches+`
		ORDER BY jobs.first_seen_at DESC, jobs.title
		LIMIT $4 OFFSET $5`, query, filter.CompanyID, status, limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobListItem, error) {
		var item JobListItem
		job, err := scanJob(row, &item.CompanyName, &item.UnseenUpdates, &item.Facts)
		item.Job = job
		item.Facts = FlattenJobFactsToJSON(item.Facts)
		return item, err
	})
	return items, total, err
}
