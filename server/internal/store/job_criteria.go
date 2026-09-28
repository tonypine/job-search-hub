package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// JobCriteria are what make a job worth the owner's time. They change as the
// search goes, so they are data the owner edits, not code.
type JobCriteria struct {
	// Roles are the job titles the owner is after, e.g. "Senior Full-Stack Engineer".
	Roles []string `json:"roles"`
	// SearchTerms are what job feeds are searched by, e.g. "react".
	SearchTerms []string `json:"search_terms"`
	// Technologies are the stack the owner wants to work in.
	Technologies []string `json:"technologies"`
	// SeniorityLevels are the levels that fit, e.g. "Senior", "Staff".
	SeniorityLevels []string `json:"seniority_levels"`
	// HomeCountry is where the owner lives and works from, e.g. "Brazil".
	// Feeds that filter by country search by it.
	HomeCountry string `json:"home_country"`
	// EligibleLocationTerms are words in a location restriction that include
	// the owner, e.g. "Brazil", "LATAM", "Americas", "worldwide".
	EligibleLocationTerms []string `json:"eligible_location_terms"`
	// IneligibleLocationTerms are words that exclude the owner even beside an
	// eligible one, e.g. "must reside in the US".
	IneligibleLocationTerms []string `json:"ineligible_location_terms"`
	// MinimumYearlyPay is the pay floor; nil applies no pay filter.
	MinimumYearlyPay *MinimumYearlyPay `json:"minimum_yearly_pay,omitempty"`
	// RefuseHourlyWork marks jobs paid by the hour as a poor fit.
	RefuseHourlyWork bool `json:"refuse_hourly_work"`
}

type MinimumYearlyPay struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// SavedJobCriteria are the criteria with the time they were last saved.
type SavedJobCriteria struct {
	Criteria  JobCriteria `json:"criteria"`
	UpdatedAt time.Time   `json:"updated_at"`
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func (criteria JobCriteria) validate() error {
	if pay := criteria.MinimumYearlyPay; pay != nil {
		if pay.Amount < 0 {
			return errors.New("the minimum yearly pay cannot be negative")
		}
		if !currencyCode.MatchString(pay.Currency) {
			return errors.New("the minimum yearly pay needs a three-letter currency code, such as USD")
		}
	}
	return nil
}

func (s *Store) GetJobCriteria(ctx context.Context) (SavedJobCriteria, error) {
	var saved SavedJobCriteria
	var body json.RawMessage
	if err := s.pool.QueryRow(ctx, `SELECT body, updated_at FROM job_criteria`).Scan(&body, &saved.UpdatedAt); err != nil {
		return SavedJobCriteria{}, err
	}
	return saved, json.Unmarshal(body, &saved.Criteria)
}

// SaveJobCriteria replaces the criteria and records their before and after.
// A save that changes nothing records nothing.
func (s *Store) SaveJobCriteria(ctx context.Context, actor Actor, criteria JobCriteria) (SavedJobCriteria, error) {
	if err := criteria.validate(); err != nil {
		return SavedJobCriteria{}, err
	}
	var saved SavedJobCriteria
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		var currentBody json.RawMessage
		if err := tx.QueryRow(ctx, `SELECT id, body, updated_at FROM job_criteria FOR UPDATE`).Scan(&id, &currentBody, &saved.UpdatedAt); err != nil {
			return err
		}
		var current JobCriteria
		if err := json.Unmarshal(currentBody, &current); err != nil {
			return err
		}
		saved.Criteria = current
		if reflect.DeepEqual(current, criteria) {
			return nil
		}
		if err := tx.QueryRow(ctx, `UPDATE job_criteria SET body = $2, updated_at = now() WHERE id = $1 RETURNING updated_at`, id, criteria).
			Scan(&saved.UpdatedAt); err != nil {
			return err
		}
		saved.Criteria = criteria
		return insertChange(ctx, tx, actor, change{entityType: "job_criteria", entityID: id, operation: "update", before: current, after: criteria})
	})
	return saved, err
}
