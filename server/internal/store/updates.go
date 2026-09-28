package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Update is something the owner should hear about: a reply, a confirmation,
// a follow-up falling due. It is about a job, a company, both, or nothing in
// particular, and is unseen until SeenAt is set.
type Update struct {
	ID          uuid.UUID  `json:"id"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Body        string     `json:"body,omitempty"`
	JobID       *uuid.UUID `json:"job_id,omitempty"`
	CompanyID   *uuid.UUID `json:"company_id,omitempty"`
	SourceURL   string     `json:"source_url,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	SeenAt      *time.Time `json:"seen_at,omitempty"`
	JobTitle    *string    `json:"job_title,omitempty"`
	CompanyName *string    `json:"company_name,omitempty"`
}

// NewUpdate is an update to record. An update about a job is also about the
// job's company.
type NewUpdate struct {
	Kind      string
	Title     string
	Body      string
	JobID     *uuid.UUID
	CompanyID *uuid.UUID
	SourceURL string
}

// UpdateList is a page of updates, newest first, with how many are unseen
// in all.
type UpdateList struct {
	Updates     []Update `json:"updates"`
	UnseenCount int      `json:"unseen_count"`
}

// UpdateSelection names the updates to mark seen: by ID, all of a job's,
// all of a company's (its jobs' included), or every one.
type UpdateSelection struct {
	IDs       []uuid.UUID `json:"ids"`
	JobID     *uuid.UUID  `json:"job_id"`
	CompanyID *uuid.UUID  `json:"company_id"`
	All       bool        `json:"all"`
}

const updateColumns = `updates.id, updates.kind, updates.title, updates.body, updates.job_id, updates.company_id, updates.source_url,
	updates.created_at, updates.seen_at, jobs.title, COALESCE(companies.name, NULLIF(jobs.company_name, ''))`

const updateJoins = `FROM updates
	LEFT JOIN jobs ON jobs.id = updates.job_id
	LEFT JOIN companies ON companies.id = updates.company_id`

func scanUpdate(row pgx.Row) (Update, error) {
	var update Update
	err := row.Scan(&update.ID, &update.Kind, &update.Title, &update.Body, &update.JobID, &update.CompanyID, &update.SourceURL,
		&update.CreatedAt, &update.SeenAt, &update.JobTitle, &update.CompanyName)
	return update, err
}

// RecordUpdate stores an unseen update. Updates are notices about the hub's
// own records, so recording one writes no change.
func (s *Store) RecordUpdate(ctx context.Context, input NewUpdate) (Update, error) {
	kind, title := strings.TrimSpace(input.Kind), strings.TrimSpace(input.Title)
	if kind == "" || title == "" {
		return Update{}, errors.New("an update needs a kind and a title")
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO updates (kind, title, body, job_id, company_id, source_url)
		VALUES ($1, $2, $3, $4, COALESCE($5, (SELECT company_id FROM jobs WHERE id = $4)), $6)
		RETURNING id`, kind, title, input.Body, input.JobID, input.CompanyID, input.SourceURL).Scan(&id)
	if isForeignKeyViolation(err) {
		return Update{}, errors.New("the update's job or company is not in the hub")
	}
	if err != nil {
		return Update{}, err
	}
	return scanUpdate(s.pool.QueryRow(ctx, `SELECT `+updateColumns+` `+updateJoins+` WHERE updates.id = $1`, id))
}

// ListUpdates returns up to limit updates, newest first; only unseen ones
// when unseenOnly.
func (s *Store) ListUpdates(ctx context.Context, limit int, unseenOnly bool) (UpdateList, error) {
	list := UpdateList{Updates: []Update{}}
	rows, err := s.pool.Query(ctx, `
		SELECT `+updateColumns+` `+updateJoins+`
		WHERE NOT $2 OR updates.seen_at IS NULL
		ORDER BY updates.created_at DESC
		LIMIT $1`, limit, unseenOnly)
	if err != nil {
		return UpdateList{}, err
	}
	if list.Updates, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Update, error) { return scanUpdate(row) }); err != nil {
		return UpdateList{}, err
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM updates WHERE seen_at IS NULL`).Scan(&list.UnseenCount)
	return list, err
}

// MarkUpdatesSeen marks the selected unseen updates seen and returns how many
// it marked.
func (s *Store) MarkUpdatesSeen(ctx context.Context, selection UpdateSelection) (int, error) {
	if len(selection.IDs) == 0 && selection.JobID == nil && selection.CompanyID == nil && !selection.All {
		return 0, errors.New("name the updates to mark seen: ids, a job, a company, or all")
	}
	ids := selection.IDs
	if ids == nil {
		ids = []uuid.UUID{}
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE updates SET seen_at = now()
		WHERE seen_at IS NULL
		  AND ($1 OR id = ANY($2) OR job_id = $3 OR company_id = $4)`,
		selection.All, ids, selection.JobID, selection.CompanyID)
	return int(tag.RowsAffected()), err
}

// Unseen counts, as columns of the queries that list jobs, companies and
// pipeline cards.
const (
	jobUnseenUpdates     = `(SELECT count(*) FROM updates WHERE updates.seen_at IS NULL AND updates.job_id = jobs.id)`
	companyUnseenUpdates = `(SELECT count(*) FROM updates WHERE updates.seen_at IS NULL AND updates.company_id = companies.id)`
	// A job's card counts its job's updates; a card for a company alone
	// counts the company's updates that are about no job.
	cardUnseenUpdates = `(SELECT count(*) FROM updates WHERE updates.seen_at IS NULL AND (
		(applications.job_id IS NOT NULL AND updates.job_id = applications.job_id) OR
		(applications.job_id IS NULL AND updates.company_id = applications.company_id AND updates.job_id IS NULL)))`
)
