package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// DecisionQueueItem is a briefed job waiting for the owner's decision: its
// brief's match and reason, the full brief's over the pre-brief.
type DecisionQueueItem struct {
	Job         Job
	CompanyName *string
	// Facts are the facts as read, which the fit is judged from.
	Facts     json.RawMessage
	Match     string
	Reason    string
	BriefTier string
	// Decision is set when the job was left for later.
	Decision *JobDecision
}

// ListDecisionQueue returns the briefed jobs to decide, open, undismissed and
// off the pipeline: the undecided ones first, then those left for later
// before laterBefore; within each, the best match first, then the newest.
func (s *Store) ListDecisionQueue(ctx context.Context, laterBefore time.Time) ([]DecisionQueueItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+prefixedJobColumns+`, COALESCE(companies.name, NULLIF(jobs.company_name, '')), job_facts.facts,
		       best.match, best.reason, best.tier, decided.decision, decided.reason, decided.decided_at
		FROM jobs
		JOIN LATERAL (SELECT match, reason, tier FROM job_briefs WHERE job_briefs.job_id = jobs.id ORDER BY tier = 'full' DESC LIMIT 1) best ON true
		LEFT JOIN job_facts ON job_facts.job_id = jobs.id
		LEFT JOIN companies ON companies.id = jobs.company_id
		LEFT JOIN job_decisions decided ON decided.job_id = jobs.id
		WHERE jobs.closed_at IS NULL AND jobs.dismissed_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM applications WHERE applications.job_id = jobs.id)
		  AND (decided.job_id IS NULL OR (decided.decision = 'later' AND decided.decided_at < $1))
		ORDER BY decided.job_id IS NOT NULL,
		         CASE best.match WHEN 'strong' THEN 0 WHEN 'possible' THEN 1 WHEN 'stretch' THEN 2 ELSE 3 END,
		         jobs.first_seen_at DESC`, laterBefore)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (DecisionQueueItem, error) {
		var item DecisionQueueItem
		var decision, decisionReason *string
		var decidedAt *time.Time
		job, err := scanJob(row, &item.CompanyName, &item.Facts, &item.Match, &item.Reason, &item.BriefTier, &decision, &decisionReason, &decidedAt)
		item.Job = job
		if decision != nil && decidedAt != nil {
			item.Decision = &JobDecision{JobID: job.ID, Decision: *decision, DecidedAt: *decidedAt}
			if decisionReason != nil {
				item.Decision.Reason = *decisionReason
			}
		}
		return item, err
	})
}

// DecisionTiming is one decision, with when its job was first seen.
type DecisionTiming struct {
	Decision    string
	DecidedAt   time.Time
	FirstSeenAt time.Time
}

// ListDecisionsSince returns the decisions made since since.
func (s *Store) ListDecisionsSince(ctx context.Context, since time.Time) ([]DecisionTiming, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT job_decisions.decision, job_decisions.decided_at, jobs.first_seen_at
		FROM job_decisions JOIN jobs ON jobs.id = job_decisions.job_id
		WHERE job_decisions.decided_at >= $1`, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[DecisionTiming])
}

// SeenJob is a job first seen in a period, with its facts and whether the
// owner has decided on it.
type SeenJob struct {
	Job       Job
	Facts     json.RawMessage
	IsDecided bool
}

// ListJobsSeenSince returns the jobs first seen since since.
func (s *Store) ListJobsSeenSince(ctx context.Context, since time.Time) ([]SeenJob, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+prefixedJobColumns+`, job_facts.facts,
		       EXISTS (SELECT 1 FROM job_decisions WHERE job_decisions.job_id = jobs.id)
		           OR EXISTS (SELECT 1 FROM applications WHERE applications.job_id = jobs.id)
		FROM jobs LEFT JOIN job_facts ON job_facts.job_id = jobs.id
		WHERE jobs.first_seen_at >= $1`, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SeenJob, error) {
		var seen SeenJob
		job, err := scanJob(row, &seen.Facts, &seen.IsDecided)
		seen.Job = job
		return seen, err
	})
}
