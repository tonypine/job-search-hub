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

// What a LinkedIn job entry records.
const (
	LinkedInJobApplied = "applied"
	LinkedInJobSaved   = "saved"
)

// recentLinkedInJobAge is how old an application or a save can be and still
// become an active card; older applications are kept as closed history.
const recentLinkedInJobAge = 90 * 24 * time.Hour

// NewLinkedInJob is a job the owner applied to or saved on LinkedIn.
type NewLinkedInJob struct {
	Kind        string
	At          time.Time
	URL         string
	Title       string
	CompanyName string
}

// LinkedInJobsImport counts what an import of applications or saved jobs did.
type LinkedInJobsImport struct {
	Active         int `json:"active"`
	Closed         int `json:"closed"`
	Skipped        int `json:"skipped"`
	AlreadyOnBoard int `json:"already_on_board"`
}

// ImportLinkedInJobs puts the owner's LinkedIn applications and saved jobs on
// the pipeline: a recent application in Applied at its date, a recent save
// in the first phase, an older application in the closed phase as history,
// and an older save not at all. An application is dated as gone out on its
// date. A job already on the board keeps its card.
func (s *Store) ImportLinkedInJobs(ctx context.Context, actor Actor, entries []NewLinkedInJob, now time.Time) (LinkedInJobsImport, error) {
	var result LinkedInJobsImport
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		phases, err := listPhasesForImport(ctx, tx)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			isRecent := now.Sub(entry.At) <= recentLinkedInJobAge
			var phaseID uuid.UUID
			closedReason := ""
			switch {
			case entry.Kind == LinkedInJobApplied && isRecent:
				phaseID = phases.applied
			case entry.Kind == LinkedInJobApplied:
				phaseID = phases.closed
				closedReason = fmt.Sprintf("Applied on LinkedIn on %s; no outcome recorded", entry.At.Format("2006-01-02"))
			case isRecent:
				phaseID = phases.first
			default:
				result.Skipped++
				continue
			}
			jobID, err := getOrAddLinkedInJob(ctx, tx, actor, entry)
			if err != nil {
				return err
			}
			if closedReason != "" {
				// A posting applied to long ago is no longer open.
				if _, err := tx.Exec(ctx, `UPDATE jobs SET closed_at = $2 WHERE id = $1 AND closed_at IS NULL`, jobID, now); err != nil {
					return err
				}
			}
			var onBoard bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM applications WHERE job_id = $1)`, jobID).Scan(&onBoard); err != nil {
				return err
			}
			if onBoard {
				result.AlreadyOnBoard++
				continue
			}
			var appliedAt *time.Time
			if entry.Kind == LinkedInJobApplied {
				appliedAt = &entry.At
			}
			application, err := scanApplication(tx.QueryRow(ctx, `
				INSERT INTO applications (job_id, phase_id, closed_reason, phase_entered_at, applied_at)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING `+applicationColumns, jobID, phaseID, closedReason, entry.At, appliedAt))
			if err != nil {
				return err
			}
			if err := insertChange(ctx, tx, actor, change{
				entityType: "application", entityID: application.ID, operation: "create", after: application, sourceURL: entry.URL,
			}); err != nil {
				return err
			}
			if closedReason != "" {
				result.Closed++
			} else {
				result.Active++
			}
		}
		_, err = tieJobsToCompanies(ctx, tx)
		return err
	})
	return result, err
}

type importPhases struct {
	first, applied, closed uuid.UUID
}

// listPhasesForImport finds the first phase, the one named Applied, and the
// closed one.
func listPhasesForImport(ctx context.Context, tx pgx.Tx) (importPhases, error) {
	rows, err := tx.Query(ctx, `SELECT `+pipelinePhaseColumns+` FROM pipeline_phases ORDER BY position`)
	if err != nil {
		return importPhases{}, err
	}
	phases, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (PipelinePhase, error) { return scanPipelinePhase(row) })
	if err != nil {
		return importPhases{}, err
	}
	var found importPhases
	for _, phase := range phases {
		if found.first == uuid.Nil {
			found.first = phase.ID
		}
		if strings.EqualFold(strings.TrimSpace(phase.Name), "applied") {
			found.applied = phase.ID
		}
		if phase.IsClosed && found.closed == uuid.Nil {
			found.closed = phase.ID
		}
	}
	if found.applied == uuid.Nil || found.closed == uuid.Nil {
		return importPhases{}, errors.New("the pipeline needs a phase named Applied and a closed phase")
	}
	return found, nil
}

// getOrAddLinkedInJob returns the job added by hand at the entry's URL,
// adding it first when there is none.
func getOrAddLinkedInJob(ctx context.Context, tx pgx.Tx, actor Actor, entry NewLinkedInJob) (uuid.UUID, error) {
	var jobID uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO jobs (source, title, url, company_name) VALUES ('manual', $1, $2, $3)
		ON CONFLICT (url) WHERE source = 'manual' DO NOTHING
		RETURNING id`, entry.Title, entry.URL, entry.CompanyName).Scan(&jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM jobs WHERE source = 'manual' AND url = $1`, entry.URL).Scan(&jobID)
		return jobID, err
	}
	if err != nil {
		return uuid.Nil, err
	}
	return jobID, insertChange(ctx, tx, actor, change{
		entityType: "job", entityID: jobID, operation: "create", after: map[string]string{"title": entry.Title, "url": entry.URL}, sourceURL: entry.URL,
	})
}
