package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The fields a fix can correct, by their column names.
const (
	JobFieldTitle          = "title"
	JobFieldCompanyName    = "company_name"
	JobFieldCompanyID      = "company_id"
	JobFieldLocation       = "location"
	JobFieldWorkplaceType  = "workplace_type"
	JobFieldEmploymentType = "employment_type"
	JobFieldPay            = "pay"
)

// JobDetailsFix corrects a job's details: each field given replaces the
// job's, for the reason given under its name in Reasons, and stays as fixed
// when the job's board or feed lists it again.
type JobDetailsFix struct {
	Title          *string
	CompanyName    *string
	CompanyID      *uuid.UUID
	Location       *string
	WorkplaceType  *string
	EmploymentType *string
	Pay            *Pay
	Reasons        map[string]string
}

// fixedField is one field a fix gives: its column, and its value before and
// after.
type fixedField struct {
	column string
	before any
	after  any
}

// getFixedFields returns the fields the fix gives, against the job's
// current values and the company name its posting gave.
func (fix JobDetailsFix) getFixedFields(current Job, currentCompanyName string) []fixedField {
	var fields []fixedField
	if fix.Title != nil {
		fields = append(fields, fixedField{JobFieldTitle, current.Title, *fix.Title})
	}
	if fix.CompanyName != nil {
		fields = append(fields, fixedField{JobFieldCompanyName, currentCompanyName, *fix.CompanyName})
	}
	if fix.CompanyID != nil {
		fields = append(fields, fixedField{JobFieldCompanyID, current.CompanyID, *fix.CompanyID})
	}
	if fix.Location != nil {
		fields = append(fields, fixedField{JobFieldLocation, current.Location, *fix.Location})
	}
	if fix.WorkplaceType != nil {
		fields = append(fields, fixedField{JobFieldWorkplaceType, current.WorkplaceType, *fix.WorkplaceType})
	}
	if fix.EmploymentType != nil {
		fields = append(fields, fixedField{JobFieldEmploymentType, current.EmploymentType, *fix.EmploymentType})
	}
	if fix.Pay != nil {
		fields = append(fields, fixedField{JobFieldPay, current.Pay, *fix.Pay})
	}
	return fields
}

// FixJobDetails applies the fix and records each corrected field as fixed,
// with a change row carrying its reason. Every field needs a reason; a
// board's job keeps its board's company.
func (s *Store) FixJobDetails(ctx context.Context, actor Actor, jobID uuid.UUID, fix JobDetailsFix) (Job, error) {
	if fix.Title != nil && strings.TrimSpace(*fix.Title) == "" {
		return Job{}, errors.New("a job's title can't be empty")
	}
	var job Job
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var currentCompanyName string
		current, err := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+`, company_name FROM jobs WHERE id = $1 FOR UPDATE`, jobID), &currentCompanyName)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJobNotFound
		}
		if err != nil {
			return err
		}
		fields := fix.getFixedFields(current, currentCompanyName)
		if len(fields) == 0 {
			return errors.New("the fix names no field to correct")
		}
		columns := make([]string, len(fields))
		for index, field := range fields {
			if strings.TrimSpace(fix.Reasons[field.column]) == "" {
				return fmt.Errorf("the fix to %s needs a reason", field.column)
			}
			columns[index] = field.column
		}
		for column := range fix.Reasons {
			if !slices.Contains(columns, column) {
				return fmt.Errorf("a reason is given for %s, which the fix doesn't change", column)
			}
		}
		if fix.CompanyID != nil && current.Source == JobSourceJobBoard {
			return ErrJobCompanyFromBoard
		}
		job, err = scanJob(tx.QueryRow(ctx, `
			UPDATE jobs SET title = COALESCE($2, title), company_name = COALESCE($3, company_name), company_id = COALESCE($4, company_id),
				location = COALESCE($5, location), workplace_type = COALESCE($6, workplace_type),
				employment_type = COALESCE($7, employment_type), pay = COALESCE($8, pay),
				fixed_fields = ARRAY(SELECT DISTINCT unnest(fixed_fields || $9::text[]) ORDER BY 1)
			WHERE id = $1 RETURNING `+jobColumns,
			jobID, fix.Title, fix.CompanyName, fix.CompanyID, fix.Location, fix.WorkplaceType, fix.EmploymentType, fix.Pay, columns))
		if isForeignKeyViolation(err) {
			return ErrCompanyNotFound
		}
		if err != nil {
			return err
		}
		if fix.CompanyID != nil {
			if _, err := tx.Exec(ctx, `UPDATE applications SET company_id = $2, updated_at = now() WHERE job_id = $1`, jobID, *fix.CompanyID); err != nil {
				return err
			}
		}
		for _, field := range fields {
			if err := insertChange(ctx, tx, actor, change{
				entityType: "job", entityID: jobID, operation: "fix",
				before: map[string]any{field.column: field.before},
				after:  map[string]any{field.column: field.after, "reason": strings.TrimSpace(fix.Reasons[field.column])},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return job, err
}

// buildPollAssignment builds a poll's SET assignment for a column a fix can
// correct: the polled value, unless the job's fix holds the column.
func buildPollAssignment(column, polledValue string) string {
	return fmt.Sprintf("%s = CASE WHEN '%s' = ANY(fixed_fields) THEN %s ELSE %s END", column, column, column, polledValue)
}
