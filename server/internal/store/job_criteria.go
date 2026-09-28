package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// TakeHome judges pay by what it would leave the owner each month; nil
	// applies no pay check.
	TakeHome *TakeHome `json:"take_home,omitempty"`
	// RefuseHourlyWork marks jobs paid by the hour as a poor fit.
	RefuseHourlyWork bool `json:"refuse_hourly_work"`
}

// TakeHome is the pay the owner needs, as monthly take-home in Currency, and
// how much of a posting's pay each way of being hired would leave.
type TakeHome struct {
	Currency string `json:"currency"`
	// MinimumMonthly is the least that pays the bills.
	MinimumMonthly float64 `json:"minimum_monthly"`
	// TargetMonthly is the take-home worth aiming for, such as the last job's.
	TargetMonthly float64 `json:"target_monthly"`
	// CLT covers Brazilian employment, including through an employer of record.
	CLT HiringTakeHome `json:"clt"`
	// PJ covers invoicing a Brazilian client as a company.
	PJ HiringTakeHome `json:"pj"`
	// ForeignContractor covers invoicing a company abroad.
	ForeignContractor HiringTakeHome `json:"foreign_contractor"`
}

// HiringTakeHome is the share of each payment one way of being hired leaves
// after taxes and fees, and how many payments a year it makes: 13.33 for CLT,
// with the 13th salary and the vacation third.
type HiringTakeHome struct {
	Share           float64 `json:"share"`
	PaymentsPerYear float64 `json:"payments_per_year"`
}

// SavedJobCriteria are the criteria with the time they were last saved.
type SavedJobCriteria struct {
	Criteria  JobCriteria `json:"criteria"`
	UpdatedAt time.Time   `json:"updated_at"`
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func (criteria JobCriteria) validate() error {
	takeHome := criteria.TakeHome
	if takeHome == nil {
		return nil
	}
	if !currencyCode.MatchString(takeHome.Currency) {
		return errors.New("the take-home needs a three-letter currency code, such as BRL")
	}
	if takeHome.MinimumMonthly < 0 || takeHome.TargetMonthly < 0 {
		return errors.New("the take-home minimum and target cannot be negative")
	}
	for name, hiring := range map[string]HiringTakeHome{"clt": takeHome.CLT, "pj": takeHome.PJ, "foreign_contractor": takeHome.ForeignContractor} {
		if hiring.Share <= 0 || hiring.Share > 1 || hiring.PaymentsPerYear <= 0 {
			return fmt.Errorf("the %s take-home needs a share above 0 and up to 1, and payments per year above 0", name)
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
