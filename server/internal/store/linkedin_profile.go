package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// LinkedInProfile is the owner's LinkedIn profile, as their export gives it.
type LinkedInProfile struct {
	Headline    string               `json:"headline,omitempty"`
	Summary     string               `json:"summary,omitempty"`
	Industry    string               `json:"industry,omitempty"`
	Location    string               `json:"location,omitempty"`
	Websites    []string             `json:"websites,omitempty"`
	Positions   []LinkedInPosition   `json:"positions,omitempty"`
	Skills      []string             `json:"skills,omitempty"`
	Education   []LinkedInEducation  `json:"education,omitempty"`
	Languages   []LinkedInLanguage   `json:"languages,omitempty"`
	Projects    []LinkedInProject    `json:"projects,omitempty"`
	Courses     []string             `json:"courses,omitempty"`
	Preferences *LinkedInPreferences `json:"preferences,omitempty"`
	UpdatedAt   *time.Time           `json:"updated_at,omitempty"`
}

type LinkedInPosition struct {
	Company     string `json:"company"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	StartedOn   string `json:"started_on,omitempty"`
	FinishedOn  string `json:"finished_on,omitempty"`
}

type LinkedInEducation struct {
	School    string `json:"school"`
	Degree    string `json:"degree,omitempty"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

type LinkedInLanguage struct {
	Name        string `json:"name"`
	Proficiency string `json:"proficiency,omitempty"`
}

type LinkedInProject struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

// LinkedInPreferences are what the owner tells LinkedIn, and recruiters, they
// are looking for.
type LinkedInPreferences struct {
	JobTitles        []string `json:"job_titles,omitempty"`
	Locations        []string `json:"locations,omitempty"`
	Industries       []string `json:"industries,omitempty"`
	JobTypes         []string `json:"job_types,omitempty"`
	CompanySize      string   `json:"company_size,omitempty"`
	OpenToRecruiters string   `json:"open_to_recruiters,omitempty"`
}

// GetLinkedInProfile returns the stored profile, empty when none was imported.
func (s *Store) GetLinkedInProfile(ctx context.Context) (LinkedInProfile, error) {
	var snapshot json.RawMessage
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, `SELECT snapshot, updated_at FROM linkedin_profile`).Scan(&snapshot, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LinkedInProfile{}, nil
	}
	if err != nil {
		return LinkedInProfile{}, err
	}
	var profile LinkedInProfile
	if err := json.Unmarshal(snapshot, &profile); err != nil {
		return LinkedInProfile{}, err
	}
	profile.UpdatedAt = &updatedAt
	return profile, nil
}

// SaveLinkedInProfile replaces the stored profile.
func (s *Store) SaveLinkedInProfile(ctx context.Context, actor Actor, profile LinkedInProfile) error {
	profile.UpdatedAt = nil
	snapshot, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO linkedin_profile (snapshot) VALUES ($1)
			ON CONFLICT (singleton) DO UPDATE SET snapshot = EXCLUDED.snapshot, updated_at = now()`, snapshot); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "linkedin_profile", entityID: uuid.New(), operation: "import",
			after: map[string]int{"positions": len(profile.Positions), "skills": len(profile.Skills)}})
	})
}
