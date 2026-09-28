package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrApplicationNotFound   = errors.New("application not found")
	ErrPipelinePhaseNotFound = errors.New("pipeline phase not found")
	ErrPipelinePhaseInUse    = errors.New("the phase still has applications; move them first")
	ErrPipelinePhaseNameUsed = errors.New("another phase already has that name")
	ErrJobNotFound           = errors.New("job not found")
)

// PipelinePhase is one column of the board. IsClosed marks the phase where
// finished applications rest, with the reason they ended.
type PipelinePhase struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Position int       `json:"position"`
	IsClosed bool      `json:"is_closed"`
}

const pipelinePhaseColumns = `id, name, position, is_closed`

func scanPipelinePhase(row pgx.Row) (PipelinePhase, error) {
	var phase PipelinePhase
	err := row.Scan(&phase.ID, &phase.Name, &phase.Position, &phase.IsClosed)
	if errors.Is(err, pgx.ErrNoRows) {
		return PipelinePhase{}, ErrPipelinePhaseNotFound
	}
	return phase, err
}

type Application struct {
	ID             uuid.UUID  `json:"id"`
	JobID          *uuid.UUID `json:"job_id,omitempty"`
	CompanyID      *uuid.UUID `json:"company_id,omitempty"`
	PhaseID        uuid.UUID  `json:"phase_id"`
	ClosedReason   string     `json:"closed_reason,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	PhaseEnteredAt time.Time  `json:"phase_entered_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

const applicationColumns = `id, job_id, company_id, phase_id, closed_reason, notes, phase_entered_at, created_at, updated_at`

func scanApplication(row pgx.Row, extra ...any) (Application, error) {
	var application Application
	destinations := append([]any{&application.ID, &application.JobID, &application.CompanyID, &application.PhaseID, &application.ClosedReason,
		&application.Notes, &application.PhaseEnteredAt, &application.CreatedAt, &application.UpdatedAt}, extra...)
	err := row.Scan(destinations...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrApplicationNotFound
	}
	return application, err
}

func (s *Store) ListPipelinePhases(ctx context.Context) ([]PipelinePhase, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+pipelinePhaseColumns+` FROM pipeline_phases ORDER BY position, name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PipelinePhase, error) { return scanPipelinePhase(row) })
}

// ApplicationInput names what goes on the board: a job, or a company for
// outreach with no posting. A job's company is taken from the job.
type ApplicationInput struct {
	JobID     *uuid.UUID
	CompanyID *uuid.UUID
	Notes     string
}

// AddApplication puts a job or company on the board in its first phase. A job
// already on the board keeps its application; created reports which.
func (s *Store) AddApplication(ctx context.Context, actor Actor, input ApplicationInput) (Application, bool, error) {
	if input.JobID == nil && input.CompanyID == nil {
		return Application{}, false, errors.New("an application needs a job or a company")
	}
	var application Application
	created := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		companyID := input.CompanyID
		if input.JobID != nil {
			var jobCompanyID *uuid.UUID
			err := tx.QueryRow(ctx, `SELECT company_id FROM jobs WHERE id = $1`, *input.JobID).Scan(&jobCompanyID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrJobNotFound
			}
			if err != nil {
				return err
			}
			if companyID == nil {
				companyID = jobCompanyID
			}
			existing, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM applications WHERE job_id = $1`, *input.JobID))
			if err == nil {
				application = existing
				return nil
			}
			if !errors.Is(err, ErrApplicationNotFound) {
				return err
			}
		}

		inserted, err := scanApplication(tx.QueryRow(ctx, `
			INSERT INTO applications (job_id, company_id, phase_id, notes)
			VALUES ($1, $2, (SELECT id FROM pipeline_phases ORDER BY position LIMIT 1), $3)
			RETURNING `+applicationColumns, input.JobID, companyID, input.Notes))
		if isForeignKeyViolation(err) {
			return ErrCompanyNotFound
		}
		if err != nil {
			return err
		}
		application = inserted
		created = true
		return insertChange(ctx, tx, actor, change{entityType: "application", entityID: application.ID, operation: "create", after: application})
	})
	return application, created, err
}

// MoveApplication puts the application in another phase and records the
// phase it left. Moving into a closed phase keeps closedReason; leaving one
// clears it.
func (s *Store) MoveApplication(ctx context.Context, actor Actor, id, phaseID uuid.UUID, closedReason string) (Application, error) {
	var application Application
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM applications WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		target, err := scanPipelinePhase(tx.QueryRow(ctx, `SELECT `+pipelinePhaseColumns+` FROM pipeline_phases WHERE id = $1`, phaseID))
		if err != nil {
			return err
		}
		if !target.IsClosed {
			closedReason = ""
		}
		application, err = scanApplication(tx.QueryRow(ctx, `
			UPDATE applications SET phase_id = $2, closed_reason = $3, phase_entered_at = now(), updated_at = now()
			WHERE id = $1
			RETURNING `+applicationColumns, id, phaseID, strings.TrimSpace(closedReason)))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "application", entityID: id, operation: "move",
			before: map[string]any{"phase_id": current.PhaseID, "closed_reason": current.ClosedReason},
			after:  map[string]any{"phase_id": application.PhaseID, "closed_reason": application.ClosedReason},
		})
	})
	return application, err
}

// UpdateApplicationNotes replaces the application's notes.
func (s *Store) UpdateApplicationNotes(ctx context.Context, actor Actor, id uuid.UUID, notes string) (Application, error) {
	var application Application
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM applications WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		application, err = scanApplication(tx.QueryRow(ctx, `
			UPDATE applications SET notes = $2, updated_at = now() WHERE id = $1 RETURNING `+applicationColumns, id, notes))
		if err != nil || current.Notes == notes {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "application", entityID: id, operation: "update",
			before: map[string]string{"notes": current.Notes}, after: map[string]string{"notes": notes},
		})
	})
	return application, err
}

// PipelineCard is one application as the board shows it.
type PipelineCard struct {
	Application Application `json:"application"`
	JobTitle    *string     `json:"job_title,omitempty"`
	JobURL      *string     `json:"job_url,omitempty"`
	CompanyName *string     `json:"company_name,omitempty"`
}

func (s *Store) ListPipelineCards(ctx context.Context) ([]PipelineCard, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT applications.id, applications.job_id, applications.company_id, applications.phase_id, applications.closed_reason,
		       applications.notes, applications.phase_entered_at, applications.created_at, applications.updated_at,
		       jobs.title, jobs.url, companies.name
		FROM applications
		LEFT JOIN jobs ON jobs.id = applications.job_id
		LEFT JOIN companies ON companies.id = applications.company_id
		ORDER BY applications.phase_entered_at DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PipelineCard, error) {
		var card PipelineCard
		application, err := scanApplication(row, &card.JobTitle, &card.JobURL, &card.CompanyName)
		card.Application = application
		return card, err
	})
}

// AddPipelinePhase appends a phase after the last one.
func (s *Store) AddPipelinePhase(ctx context.Context, actor Actor, name string, isClosed bool) (PipelinePhase, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return PipelinePhase{}, errors.New("a phase needs a name")
	}
	var phase PipelinePhase
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		phase, err = scanPipelinePhase(tx.QueryRow(ctx, `
			INSERT INTO pipeline_phases (name, position, is_closed)
			SELECT $1, COALESCE(MAX(position), 0) + 1, $2 FROM pipeline_phases
			RETURNING `+pipelinePhaseColumns, name, isClosed))
		if isUniqueViolation(err) {
			return ErrPipelinePhaseNameUsed
		}
		if err != nil {
			return fmt.Errorf("add the phase %q: %w", name, err)
		}
		return insertChange(ctx, tx, actor, change{entityType: "pipeline_phase", entityID: phase.ID, operation: "create", after: phase})
	})
	return phase, err
}

func (s *Store) RenamePipelinePhase(ctx context.Context, actor Actor, id uuid.UUID, name string) (PipelinePhase, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return PipelinePhase{}, errors.New("a phase needs a name")
	}
	var phase PipelinePhase
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanPipelinePhase(tx.QueryRow(ctx, `SELECT `+pipelinePhaseColumns+` FROM pipeline_phases WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		phase, err = scanPipelinePhase(tx.QueryRow(ctx, `UPDATE pipeline_phases SET name = $2 WHERE id = $1 RETURNING `+pipelinePhaseColumns, id, name))
		if isUniqueViolation(err) {
			return ErrPipelinePhaseNameUsed
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "pipeline_phase", entityID: id, operation: "rename",
			before: map[string]string{"name": current.Name}, after: map[string]string{"name": phase.Name},
		})
	})
	return phase, err
}

// ReorderPipelinePhases sets the phases' order; orderedIDs must name every
// phase exactly once.
func (s *Store) ReorderPipelinePhases(ctx context.Context, actor Actor, orderedIDs []uuid.UUID) ([]PipelinePhase, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var phaseCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pipeline_phases`).Scan(&phaseCount); err != nil {
			return err
		}
		distinct := map[uuid.UUID]bool{}
		for _, id := range orderedIDs {
			distinct[id] = true
		}
		if len(orderedIDs) != phaseCount || len(distinct) != phaseCount {
			return errors.New("the new order must name every phase exactly once")
		}
		for index, id := range orderedIDs {
			tag, err := tx.Exec(ctx, `UPDATE pipeline_phases SET position = $2 WHERE id = $1`, id, index+1)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrPipelinePhaseNotFound
			}
			if err := insertChange(ctx, tx, actor, change{entityType: "pipeline_phase", entityID: id, operation: "reorder", after: map[string]int{"position": index + 1}}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListPipelinePhases(ctx)
}

// DeletePipelinePhase removes an empty phase. A phase with applications, or
// the last phase left, cannot be deleted.
func (s *Store) DeletePipelinePhase(ctx context.Context, actor Actor, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		phase, err := scanPipelinePhase(tx.QueryRow(ctx, `SELECT `+pipelinePhaseColumns+` FROM pipeline_phases WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		var applicationCount, phaseCount int
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM applications WHERE phase_id = $1), (SELECT count(*) FROM pipeline_phases)`, id).
			Scan(&applicationCount, &phaseCount); err != nil {
			return err
		}
		if applicationCount > 0 {
			return ErrPipelinePhaseInUse
		}
		if phaseCount == 1 {
			return errors.New("the board needs at least one phase")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM pipeline_phases WHERE id = $1`, id); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "pipeline_phase", entityID: id, operation: "delete", before: phase})
	})
}
