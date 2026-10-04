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
	// FollowUpDays is how long a card may sit in the phase, since it entered
	// or was last followed up, before a follow-up is due; nil never falls due.
	FollowUpDays *int `json:"follow_up_days,omitempty"`
}

const pipelinePhaseColumns = `id, name, position, is_closed, follow_up_days`

func scanPipelinePhase(row pgx.Row) (PipelinePhase, error) {
	var phase PipelinePhase
	err := row.Scan(&phase.ID, &phase.Name, &phase.Position, &phase.IsClosed, &phase.FollowUpDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return PipelinePhase{}, ErrPipelinePhaseNotFound
	}
	return phase, err
}

type Application struct {
	ID               uuid.UUID  `json:"id"`
	JobID            *uuid.UUID `json:"job_id,omitempty"`
	CompanyID        *uuid.UUID `json:"company_id,omitempty"`
	PhaseID          uuid.UUID  `json:"phase_id"`
	ClosedReason     string     `json:"closed_reason,omitempty"`
	Notes            string     `json:"notes,omitempty"`
	PhaseEnteredAt   time.Time  `json:"phase_entered_at"`
	LastFollowedUpAt *time.Time `json:"last_followed_up_at,omitempty"`
	// ContactedAt is when a person at the company first wrote back.
	ContactedAt *time.Time `json:"contacted_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

const applicationColumns = `id, job_id, company_id, phase_id, closed_reason, notes, phase_entered_at, last_followed_up_at, contacted_at,
	created_at, updated_at`

func scanApplication(row pgx.Row, extra ...any) (Application, error) {
	var application Application
	destinations := append([]any{&application.ID, &application.JobID, &application.CompanyID, &application.PhaseID, &application.ClosedReason,
		&application.Notes, &application.PhaseEnteredAt, &application.LastFollowedUpAt, &application.ContactedAt, &application.CreatedAt,
		&application.UpdatedAt}, extra...)
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
		var err error
		application, created, err = addApplicationInTransaction(ctx, tx, actor, input)
		return err
	})
	return application, created, err
}

// addApplicationInTransaction is AddApplication inside tx.
func addApplicationInTransaction(ctx context.Context, tx pgx.Tx, actor Actor, input ApplicationInput) (Application, bool, error) {
	companyID := input.CompanyID
	if input.JobID != nil {
		var jobCompanyID *uuid.UUID
		err := tx.QueryRow(ctx, `SELECT company_id FROM jobs WHERE id = $1`, *input.JobID).Scan(&jobCompanyID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Application{}, false, ErrJobNotFound
		}
		if err != nil {
			return Application{}, false, err
		}
		if companyID == nil {
			companyID = jobCompanyID
		}
		existing, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM applications WHERE job_id = $1`, *input.JobID))
		if err == nil {
			return existing, false, nil
		}
		if !errors.Is(err, ErrApplicationNotFound) {
			return Application{}, false, err
		}
	}

	application, err := scanApplication(tx.QueryRow(ctx, `
		INSERT INTO applications (job_id, company_id, phase_id, notes)
		VALUES ($1, $2, (SELECT id FROM pipeline_phases ORDER BY position LIMIT 1), $3)
		RETURNING `+applicationColumns, input.JobID, companyID, input.Notes))
	if isForeignKeyViolation(err) {
		return Application{}, false, ErrCompanyNotFound
	}
	if err != nil {
		return Application{}, false, err
	}
	err = insertChange(ctx, tx, actor, change{entityType: "application", entityID: application.ID, operation: "create", after: application})
	return application, true, err
}

// MoveApplication puts the application in another phase and records the
// phase it left. Moving into a closed phase keeps closedReason; leaving one
// clears it.
func (s *Store) MoveApplication(ctx context.Context, actor Actor, id, phaseID uuid.UUID, closedReason string) (Application, error) {
	return s.MoveApplicationAsOf(ctx, actor, id, phaseID, closedReason, time.Now(), "")
}

// MoveApplicationAsOf moves the application as MoveApplication does, entering
// the phase at enteredAt, such as the date of the mail that moved it.
// sourceURL is what moved it, for the change log.
func (s *Store) MoveApplicationAsOf(ctx context.Context, actor Actor, id, phaseID uuid.UUID, closedReason string, enteredAt time.Time, sourceURL string) (Application, error) {
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
			UPDATE applications SET phase_id = $2, closed_reason = $3, phase_entered_at = $4, updated_at = now()
			WHERE id = $1
			RETURNING `+applicationColumns, id, phaseID, strings.TrimSpace(closedReason), enteredAt))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "application", entityID: id, operation: "move", sourceURL: sourceURL,
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

// cardFollowUpDueAt is when a card's phase wants a follow-up: its interval
// after the card entered the phase or was last followed up, whichever came
// later; null when the phase asks for none. It needs pipeline_phases joined.
const cardFollowUpDueAt = `GREATEST(applications.phase_entered_at, COALESCE(applications.last_followed_up_at, applications.phase_entered_at))
	+ make_interval(days => pipeline_phases.follow_up_days)`

// cardDismissedAt is a card's dismissal: its job's, or its own when it has no job.
const cardDismissedAt = `CASE WHEN applications.job_id IS NOT NULL THEN jobs.dismissed_at ELSE applications.dismissed_at END`

// PipelineCard is one application as the board shows it.
type PipelineCard struct {
	Application Application `json:"application"`
	JobTitle    *string     `json:"job_title,omitempty"`
	JobURL      *string     `json:"job_url,omitempty"`
	CompanyName *string     `json:"company_name,omitempty"`
	// FollowUpDueAt is when the card's phase wants a follow-up; nil when the
	// phase asks for none.
	FollowUpDueAt *time.Time `json:"follow_up_due_at,omitempty"`
	UnseenUpdates int        `json:"unseen_updates"`
	// DismissedAt is when the card was dismissed as not a good fit: its job's
	// dismissal, or its own when it has no job.
	DismissedAt     *time.Time `json:"dismissed_at,omitempty"`
	DismissalReason string     `json:"dismissal_reason,omitempty"`
}

// ListPipelineCards returns the cards on the board, leaving dismissed ones out.
func (s *Store) ListPipelineCards(ctx context.Context) ([]PipelineCard, error) {
	return s.listPipelineCards(ctx, false, nil)
}

// ListDismissedPipelineCards returns the cards dismissed as not a good fit,
// in the phases they left.
func (s *Store) ListDismissedPipelineCards(ctx context.Context) ([]PipelineCard, error) {
	return s.listPipelineCards(ctx, true, nil)
}

// CompanyApplication is one of a company's cards with the phase it sits in.
type CompanyApplication struct {
	PipelineCard
	PhaseName     string `json:"phase_name"`
	PhaseIsClosed bool   `json:"phase_is_closed"`
}

// ListCompanyApplications returns a company's cards on the board, newest in
// their phase first, leaving dismissed ones out.
func (s *Store) ListCompanyApplications(ctx context.Context, companyID uuid.UUID) ([]CompanyApplication, error) {
	cards, err := s.listPipelineCards(ctx, false, &companyID)
	if err != nil {
		return nil, err
	}
	phases, err := s.ListPipelinePhases(ctx)
	if err != nil {
		return nil, err
	}
	phasesByID := map[uuid.UUID]PipelinePhase{}
	for _, phase := range phases {
		phasesByID[phase.ID] = phase
	}
	applications := make([]CompanyApplication, 0, len(cards))
	for _, card := range cards {
		phase := phasesByID[card.Application.PhaseID]
		applications = append(applications, CompanyApplication{PipelineCard: card, PhaseName: phase.Name, PhaseIsClosed: phase.IsClosed})
	}
	return applications, nil
}

// listPipelineCards lists the dismissed cards or the others, all or one
// company's.
func (s *Store) listPipelineCards(ctx context.Context, dismissed bool, companyID *uuid.UUID) ([]PipelineCard, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT applications.id, applications.job_id, applications.company_id, applications.phase_id, applications.closed_reason,
		       applications.notes, applications.phase_entered_at, applications.last_followed_up_at, applications.contacted_at, applications.created_at,
		       applications.updated_at,
		       jobs.title, jobs.url, COALESCE(companies.name, NULLIF(jobs.company_name, '')),
		       `+cardFollowUpDueAt+`, `+cardUnseenUpdates+`, `+cardDismissedAt+`,
		       CASE WHEN applications.job_id IS NOT NULL THEN jobs.dismissal_reason ELSE applications.dismissal_reason END
		FROM applications
		JOIN pipeline_phases ON pipeline_phases.id = applications.phase_id
		LEFT JOIN jobs ON jobs.id = applications.job_id
		LEFT JOIN companies ON companies.id = applications.company_id
		WHERE (`+cardDismissedAt+` IS NOT NULL) = $1 AND ($2::uuid IS NULL OR applications.company_id = $2)
		ORDER BY applications.phase_entered_at DESC`, dismissed, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PipelineCard, error) {
		var card PipelineCard
		application, err := scanApplication(row, &card.JobTitle, &card.JobURL, &card.CompanyName, &card.FollowUpDueAt, &card.UnseenUpdates,
			&card.DismissedAt, &card.DismissalReason)
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

// RecordFollowUp notes that the owner followed up on the application now,
// which restarts its phase's follow-up count. The note goes to the change log.
func (s *Store) RecordFollowUp(ctx context.Context, actor Actor, id uuid.UUID, note string) (Application, error) {
	return s.RecordFollowUpAsOf(ctx, actor, id, note, time.Now(), "")
}

// RecordFollowUpAsOf notes a follow-up made at followedUpAt, such as a
// message sent then; an earlier one than the last known changes nothing.
// sourceURL is where it was made, for the change log.
func (s *Store) RecordFollowUpAsOf(ctx context.Context, actor Actor, id uuid.UUID, note string, followedUpAt time.Time, sourceURL string) (Application, error) {
	var application Application
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		application, err = scanApplication(tx.QueryRow(ctx, `
			UPDATE applications SET last_followed_up_at = GREATEST(last_followed_up_at, $2), updated_at = now() WHERE id = $1
			RETURNING `+applicationColumns, id, followedUpAt))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "application", entityID: id, operation: "follow_up", sourceURL: sourceURL,
			after: map[string]any{"note": strings.TrimSpace(note), "followed_up_at": followedUpAt},
		})
	})
	return application, err
}

// FindCompanyApplication returns the company's open application updated
// last, or its closed one updated last when it has no open one.
func (s *Store) FindCompanyApplication(ctx context.Context, companyID uuid.UUID) (Application, error) {
	return scanApplication(s.pool.QueryRow(ctx, `
		SELECT `+applicationColumns+` FROM applications WHERE id = (
			SELECT a.id FROM applications a JOIN pipeline_phases p ON p.id = a.phase_id
			WHERE a.company_id = $1 ORDER BY p.is_closed, a.updated_at DESC LIMIT 1)`, companyID))
}

// CorrectApplicationPhaseEnteredAt moves when the application entered its
// phase back to enteredAt, such as the date of the mail confirming it; a
// later date changes nothing.
func (s *Store) CorrectApplicationPhaseEnteredAt(ctx context.Context, actor Actor, id uuid.UUID, enteredAt time.Time, sourceURL string) (Application, bool, error) {
	var application Application
	corrected := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM applications WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		application = current
		if !current.PhaseEnteredAt.After(enteredAt) {
			return nil
		}
		if application, err = scanApplication(tx.QueryRow(ctx, `
			UPDATE applications SET phase_entered_at = $2, updated_at = now() WHERE id = $1 RETURNING `+applicationColumns, id, enteredAt)); err != nil {
			return err
		}
		corrected = true
		return insertChange(ctx, tx, actor, change{
			entityType: "application", entityID: id, operation: "update", sourceURL: sourceURL,
			before: map[string]any{"phase_entered_at": current.PhaseEnteredAt}, after: map[string]any{"phase_entered_at": enteredAt},
		})
	})
	return application, corrected, err
}

// MarkApplicationContacted records that a person at the company wrote back
// at contactedAt; the earliest such time is kept.
func (s *Store) MarkApplicationContacted(ctx context.Context, actor Actor, id uuid.UUID, contactedAt time.Time, sourceURL string) (Application, error) {
	var application Application
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM applications WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		application = current
		if current.ContactedAt != nil && !current.ContactedAt.After(contactedAt) {
			return nil
		}
		if application, err = scanApplication(tx.QueryRow(ctx, `
			UPDATE applications SET contacted_at = $2, updated_at = now() WHERE id = $1 RETURNING `+applicationColumns, id, contactedAt)); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "application", entityID: id, operation: "contact", sourceURL: sourceURL,
			before: map[string]any{"contacted_at": current.ContactedAt}, after: map[string]any{"contacted_at": contactedAt},
		})
	})
	return application, err
}

// SetPipelinePhaseFollowUpDays sets how many days a card may sit in the phase
// before a follow-up is due; nil stops the phase asking for one.
func (s *Store) SetPipelinePhaseFollowUpDays(ctx context.Context, actor Actor, id uuid.UUID, days *int) (PipelinePhase, error) {
	if days != nil && *days <= 0 {
		return PipelinePhase{}, errors.New("a follow-up is due after at least one day")
	}
	var phase PipelinePhase
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanPipelinePhase(tx.QueryRow(ctx, `SELECT `+pipelinePhaseColumns+` FROM pipeline_phases WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		phase, err = scanPipelinePhase(tx.QueryRow(ctx, `UPDATE pipeline_phases SET follow_up_days = $2 WHERE id = $1 RETURNING `+pipelinePhaseColumns, id, days))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "pipeline_phase", entityID: id, operation: "set_follow_up_days",
			before: map[string]*int{"follow_up_days": current.FollowUpDays}, after: map[string]*int{"follow_up_days": phase.FollowUpDays},
		})
	})
	return phase, err
}
