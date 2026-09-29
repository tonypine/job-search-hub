package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrClaudeSessionNotFound = errors.New("claude session not found")

// ClaudeSession is a Claude session about one company or one job. Its Claude
// session ID is chosen when it is created, so it can always be resumed.
// Whether it is running is not recorded here: the app's processes say so.
type ClaudeSession struct {
	ID        uuid.UUID  `json:"id"`
	CompanyID *uuid.UUID `json:"company_id,omitempty"`
	JobID     *uuid.UUID `json:"job_id,omitempty"`
	// AboutProfile marks the interview that deepens the owner's knowledge base.
	AboutProfile    bool       `json:"about_profile,omitempty"`
	ClaudeSessionID uuid.UUID  `json:"claude_session_id"`
	Name            string     `json:"name"`
	CreatedAt       time.Time  `json:"created_at"`
	LastStartedAt   *time.Time `json:"last_started_at,omitempty"`
	LastStoppedAt   *time.Time `json:"last_stopped_at,omitempty"`
}

const claudeSessionColumns = `id, company_id, job_id, about_profile, claude_session_id, name, created_at, last_started_at, last_stopped_at`

func scanClaudeSession(row pgx.Row) (ClaudeSession, error) {
	var session ClaudeSession
	err := row.Scan(&session.ID, &session.CompanyID, &session.JobID, &session.AboutProfile, &session.ClaudeSessionID, &session.Name,
		&session.CreatedAt, &session.LastStartedAt, &session.LastStoppedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClaudeSession{}, ErrClaudeSessionNotFound
	}
	return session, err
}

// ClaudeSessionSubject names what a session is about: a company, a job, or
// the owner's profile.
type ClaudeSessionSubject struct {
	CompanyID    *uuid.UUID
	JobID        *uuid.UUID
	AboutProfile bool
}

// profileSessionName names every profile interview.
const profileSessionName = "Enhance profile"

// CreateClaudeSession adds a session about the subject, named after it: the
// company's name, the job's title and company, or "Enhance profile".
func (s *Store) CreateClaudeSession(ctx context.Context, actor Actor, subject ClaudeSessionSubject) (ClaudeSession, error) {
	subjectCount := 0
	for _, isSet := range []bool{subject.CompanyID != nil, subject.JobID != nil, subject.AboutProfile} {
		if isSet {
			subjectCount++
		}
	}
	if subjectCount != 1 {
		return ClaudeSession{}, errors.New("a session is about one company, one job or the profile")
	}
	var session ClaudeSession
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		name := profileSessionName
		var err error
		if subject.CompanyID != nil {
			err = tx.QueryRow(ctx, `SELECT name FROM companies WHERE id = $1`, *subject.CompanyID).Scan(&name)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCompanyNotFound
			}
		} else if subject.JobID != nil {
			err = tx.QueryRow(ctx, `
				SELECT jobs.title || COALESCE(' · ' || COALESCE(companies.name, NULLIF(jobs.company_name, '')), '')
				FROM jobs LEFT JOIN companies ON companies.id = jobs.company_id WHERE jobs.id = $1`, *subject.JobID).Scan(&name)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrJobNotFound
			}
		}
		if err != nil {
			return err
		}
		session, err = scanClaudeSession(tx.QueryRow(ctx, `
			INSERT INTO claude_sessions (company_id, job_id, about_profile, name) VALUES ($1, $2, $3, $4)
			RETURNING `+claudeSessionColumns, subject.CompanyID, subject.JobID, subject.AboutProfile, name))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "claude_session", entityID: session.ID, operation: "create",
			after: map[string]any{"name": session.Name, "company_id": session.CompanyID, "job_id": session.JobID, "about_profile": session.AboutProfile},
		})
	})
	return session, err
}

func (s *Store) GetClaudeSession(ctx context.Context, id uuid.UUID) (ClaudeSession, error) {
	return scanClaudeSession(s.pool.QueryRow(ctx, `SELECT `+claudeSessionColumns+` FROM claude_sessions WHERE id = $1`, id))
}

// ListClaudeSessions returns up to limit sessions about the subject, or about
// anything when the subject names nothing, most recently active first.
func (s *Store) ListClaudeSessions(ctx context.Context, subject ClaudeSessionSubject, limit int) ([]ClaudeSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+claudeSessionColumns+` FROM claude_sessions
		WHERE ($1::uuid IS NULL OR company_id = $1) AND ($2::uuid IS NULL OR job_id = $2) AND (NOT $4 OR about_profile)
		ORDER BY GREATEST(created_at, COALESCE(last_started_at, created_at), COALESCE(last_stopped_at, created_at)) DESC
		LIMIT $3`, subject.CompanyID, subject.JobID, limit, subject.AboutProfile)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ClaudeSession, error) { return scanClaudeSession(row) })
}

// RecordClaudeSessionStart notes that the app started or resumed the session.
// Starts and stops are timestamps on the session, not changes.
func (s *Store) RecordClaudeSessionStart(ctx context.Context, id uuid.UUID) (ClaudeSession, error) {
	return scanClaudeSession(s.pool.QueryRow(ctx, `
		UPDATE claude_sessions SET last_started_at = now() WHERE id = $1 RETURNING `+claudeSessionColumns, id))
}

// RecordClaudeSessionStop notes that the session's process ended.
func (s *Store) RecordClaudeSessionStop(ctx context.Context, id uuid.UUID) (ClaudeSession, error) {
	return scanClaudeSession(s.pool.QueryRow(ctx, `
		UPDATE claude_sessions SET last_stopped_at = now() WHERE id = $1 RETURNING `+claudeSessionColumns, id))
}
