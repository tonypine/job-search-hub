package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	JobBriefTierPre  = "pre"
	JobBriefTierFull = "full"
)

// JobBriefPoint is one strength or weakness, with the knowledge base entries
// it rests on.
type JobBriefPoint struct {
	Point    string      `json:"point"`
	EntryIDs []uuid.UUID `json:"entry_ids"`
}

// CitedEntry is a knowledge base entry a brief cites, as the brief shows it.
type CitedEntry struct {
	ID           uuid.UUID `json:"id"`
	Kind         string    `json:"kind"`
	Title        string    `json:"title"`
	Organization string    `json:"organization,omitempty"`
}

// JobBrief is what a job's brief says: how well the owner matches it, why,
// and the strengths and weaknesses behind that, citing the knowledge base.
type JobBrief struct {
	JobID    uuid.UUID `json:"job_id"`
	Tier     string    `json:"tier"`
	PromptID uuid.UUID `json:"prompt_id"`
	Model    string    `json:"model"`
	// Match is strong, possible, stretch or mismatch.
	Match         string          `json:"match"`
	Reason        string          `json:"reason"`
	Strengths     []JobBriefPoint `json:"strengths"`
	Weaknesses    []JobBriefPoint `json:"weaknesses"`
	KnowledgeHash string          `json:"-"`
	WrittenAt     time.Time       `json:"written_at"`
	// IsStale says the knowledge base, profile or criteria changed since the
	// brief was written.
	IsStale bool `json:"is_stale"`
	// CitedEntries are the entries the points cite that still exist.
	CitedEntries []CitedEntry `json:"cited_entries"`
}

// knowledgeHash is the knowledge base, the owner's profile and the criteria
// as briefs read them: any entry added, changed or deleted changes it.
const knowledgeHash = `md5(
	COALESCE((SELECT string_agg(id::text || updated_at::text, ',' ORDER BY id) FROM profile_entries), '') ||
	COALESCE((SELECT string_agg(updated_at::text, ',') FROM owner_profile), '') ||
	COALESCE((SELECT string_agg(updated_at::text, ',') FROM job_criteria), ''))`

// GetKnowledgeHash returns what a brief written now is written against.
func (s *Store) GetKnowledgeHash(ctx context.Context) (string, error) {
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT `+knowledgeHash).Scan(&hash)
	return hash, err
}

// SaveJobBrief keeps the brief as its job's brief of its tier, replacing the
// one before.
func (s *Store) SaveJobBrief(ctx context.Context, brief JobBrief) error {
	strengths, err := json.Marshal(copyPointsForStorage(brief.Strengths))
	if err != nil {
		return err
	}
	weaknesses, err := json.Marshal(copyPointsForStorage(brief.Weaknesses))
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO job_briefs (job_id, tier, prompt_id, model, match, reason, strengths, weaknesses, knowledge_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (job_id, tier) DO UPDATE SET
			prompt_id = EXCLUDED.prompt_id, model = EXCLUDED.model, match = EXCLUDED.match, reason = EXCLUDED.reason,
			strengths = EXCLUDED.strengths, weaknesses = EXCLUDED.weaknesses, knowledge_hash = EXCLUDED.knowledge_hash, written_at = now()`,
		brief.JobID, brief.Tier, brief.PromptID, brief.Model, brief.Match, brief.Reason, strengths, weaknesses, brief.KnowledgeHash)
	if isForeignKeyViolation(err) {
		return ErrJobNotFound
	}
	return err
}

// GetJobBrief returns the job's full brief, or else its pre-brief, with the
// entries it cites; nil when the job has neither.
func (s *Store) GetJobBrief(ctx context.Context, jobID uuid.UUID) (*JobBrief, error) {
	var brief JobBrief
	var strengths, weaknesses json.RawMessage
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, tier, prompt_id, model, match, reason, strengths, weaknesses, knowledge_hash, written_at, knowledge_hash <> `+knowledgeHash+`
		FROM job_briefs WHERE job_id = $1
		ORDER BY tier = 'full' DESC LIMIT 1`, jobID).Scan(&brief.JobID, &brief.Tier, &brief.PromptID, &brief.Model, &brief.Match, &brief.Reason,
		&strengths, &weaknesses, &brief.KnowledgeHash, &brief.WrittenAt, &brief.IsStale)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(strengths, &brief.Strengths); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(weaknesses, &brief.Weaknesses); err != nil {
		return nil, err
	}
	var citedIDs []uuid.UUID
	for _, point := range append(append([]JobBriefPoint{}, brief.Strengths...), brief.Weaknesses...) {
		citedIDs = append(citedIDs, point.EntryIDs...)
	}
	rows, err := s.pool.Query(ctx, `SELECT id, kind, title, organization FROM profile_entries WHERE id = ANY($1) ORDER BY kind, title`, citedIDs)
	if err != nil {
		return nil, err
	}
	brief.CitedEntries, err = pgx.CollectRows(rows, pgx.RowToStructByPos[CitedEntry])
	return &brief, err
}

// copyPointsForStorage copies the points with every missing list made
// empty, so the stored JSON always holds arrays.
func copyPointsForStorage(points []JobBriefPoint) []JobBriefPoint {
	copied := make([]JobBriefPoint, 0, len(points))
	for _, point := range points {
		if point.EntryIDs == nil {
			point.EntryIDs = []uuid.UUID{}
		}
		copied = append(copied, point)
	}
	return copied
}

// JobToBrief is an open, undismissed job with facts whose pre-brief is
// missing, or was written by another prompt version or knowledge base.
type JobToBrief struct {
	Job         Job
	CompanyName *string
	// Facts are the facts as read, which the fit is judged from.
	Facts json.RawMessage
	// HasOutdatedBrief says a pre-brief exists but is out of date.
	HasOutdatedBrief bool
}

// ListJobsToBrief returns the jobs whose pre-brief the prompt should write:
// the ones without one first, each group newest first.
func (s *Store) ListJobsToBrief(ctx context.Context, promptID uuid.UUID, knowledgeHash string) ([]JobToBrief, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+jobToBriefColumns+`, job_briefs.job_id IS NOT NULL
		FROM jobs
		JOIN job_facts ON job_facts.job_id = jobs.id
		LEFT JOIN companies ON companies.id = jobs.company_id
		LEFT JOIN job_briefs ON job_briefs.job_id = jobs.id AND job_briefs.tier = 'pre'
		WHERE jobs.closed_at IS NULL AND jobs.dismissed_at IS NULL
		  AND (job_briefs.job_id IS NULL OR job_briefs.prompt_id <> $1 OR job_briefs.knowledge_hash <> $2)
		ORDER BY job_briefs.job_id IS NOT NULL, jobs.first_seen_at DESC`, promptID, knowledgeHash)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobToBrief, error) {
		var hasOutdatedBrief bool
		toBrief, err := scanJobToBrief(row, &hasOutdatedBrief)
		toBrief.HasOutdatedBrief = hasOutdatedBrief
		return toBrief, err
	})
}

const jobToBriefColumns = prefixedJobColumns + `, COALESCE(companies.name, NULLIF(jobs.company_name, '')), job_facts.facts`

func scanJobToBrief(row pgx.Row, extra ...any) (JobToBrief, error) {
	var toBrief JobToBrief
	job, err := scanJob(row, append([]any{&toBrief.CompanyName, &toBrief.Facts}, extra...)...)
	toBrief.Job = job
	return toBrief, err
}

// GetJobToBrief returns one job as a brief reads it, with its facts when
// they've been read.
func (s *Store) GetJobToBrief(ctx context.Context, jobID uuid.UUID) (JobToBrief, error) {
	toBrief, err := scanJobToBrief(s.pool.QueryRow(ctx, `
		SELECT `+jobToBriefColumns+`
		FROM jobs LEFT JOIN job_facts ON job_facts.job_id = jobs.id LEFT JOIN companies ON companies.id = jobs.company_id
		WHERE jobs.id = $1`, jobID))
	if errors.Is(err, pgx.ErrNoRows) {
		return JobToBrief{}, ErrJobNotFound
	}
	return toBrief, err
}

// ListJobsForFullBriefs returns up to limit of the best pre-briefed jobs that
// are still open, undismissed and off the pipeline, and whose full brief is
// missing or outdated: strong matches first, then possible ones, each the
// most recently pre-briefed first.
func (s *Store) ListJobsForFullBriefs(ctx context.Context, promptID uuid.UUID, knowledgeHash string, limit int) ([]JobToBrief, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+jobToBriefColumns+`
		FROM jobs
		JOIN job_briefs pre ON pre.job_id = jobs.id AND pre.tier = 'pre' AND pre.match IN ('strong', 'possible')
		LEFT JOIN job_briefs full_brief ON full_brief.job_id = jobs.id AND full_brief.tier = 'full'
		LEFT JOIN job_facts ON job_facts.job_id = jobs.id
		LEFT JOIN companies ON companies.id = jobs.company_id
		WHERE jobs.closed_at IS NULL AND jobs.dismissed_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM applications WHERE applications.job_id = jobs.id)
		  AND (full_brief.job_id IS NULL OR full_brief.prompt_id <> $1 OR full_brief.knowledge_hash <> $2)
		ORDER BY pre.match = 'strong' DESC, pre.written_at DESC
		LIMIT $3`, promptID, knowledgeHash, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobToBrief, error) { return scanJobToBrief(row) })
}
