package store

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// JobDetails is everything the hub knows about one job: the job with its
// board facts, its company, the facts read from its text, and its place on
// the pipeline.
type JobDetails struct {
	Job         Job               `json:"job"`
	CompanyName *string           `json:"company_name,omitempty"`
	Facts       *LabelledJobFacts `json:"facts,omitempty"`
	Application *Application      `json:"application,omitempty"`
	Phase       *PipelinePhase    `json:"phase,omitempty"`
	// UnseenUpdates counts the job's unseen updates.
	UnseenUpdates int `json:"unseen_updates"`
	// Connections are the owner's connections at the job's company, whether
	// the hub knows the company or only the posting's company name.
	Connections []Connection `json:"connections"`
	// RawFacts are the facts as read, which the fit is judged from.
	RawFacts json.RawMessage `json:"-"`
}

// LabelledJobFacts are a job's facts, each labelled by the schema of the
// prompt version they were read under, in the schema's order.
type LabelledJobFacts struct {
	Entries       []JobFactEntry `json:"entries"`
	PromptVersion int            `json:"prompt_version"`
	Model         string         `json:"model"`
	ExtractedAt   time.Time      `json:"extracted_at"`
}

// JobFactEntry is one fact: its key, the schema's title and description for
// it, the value read, and the evidence for it.
type JobFactEntry struct {
	Key         string          `json:"key"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Value       json.RawMessage `json:"value"`
	// Evidence is the posting's words the fact rests on, when it was read
	// with them.
	Evidence string `json:"evidence,omitempty"`
}

func (s *Store) GetJobDetails(ctx context.Context, id uuid.UUID) (JobDetails, error) {
	var details JobDetails
	job, err := scanJob(s.pool.QueryRow(ctx, `
		SELECT `+prefixedJobColumns+`, COALESCE(companies.name, NULLIF(jobs.company_name, '')), `+jobUnseenUpdates+`
		FROM jobs LEFT JOIN companies ON companies.id = jobs.company_id
		WHERE jobs.id = $1`, id), &details.CompanyName, &details.UnseenUpdates)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobDetails{}, ErrJobNotFound
	}
	if err != nil {
		return JobDetails{}, err
	}
	details.Job = job

	var facts LabelledJobFacts
	var rawFacts, schema json.RawMessage
	err = s.pool.QueryRow(ctx, `
		SELECT job_facts.facts, agent_prompts.result_schema, agent_prompts.version, job_facts.model, job_facts.extracted_at
		FROM job_facts JOIN agent_prompts ON agent_prompts.id = job_facts.prompt_id
		WHERE job_facts.job_id = $1`, id).Scan(&rawFacts, &schema, &facts.PromptVersion, &facts.Model, &facts.ExtractedAt)
	switch {
	case err == nil:
		if facts.Entries, err = labelJobFacts(rawFacts, schema); err != nil {
			return JobDetails{}, err
		}
		details.Facts = &facts
		details.RawFacts = rawFacts
	case !errors.Is(err, pgx.ErrNoRows):
		return JobDetails{}, err
	}

	var phase PipelinePhase
	application, err := scanApplication(s.pool.QueryRow(ctx, `
		SELECT applications.id, applications.job_id, applications.company_id, applications.phase_id, applications.closed_reason,
		       applications.notes, applications.phase_entered_at, applications.last_followed_up_at, applications.contacted_at, applications.created_at,
		       applications.updated_at,
		       pipeline_phases.id, pipeline_phases.name, pipeline_phases.position, pipeline_phases.is_closed, pipeline_phases.follow_up_days
		FROM applications JOIN pipeline_phases ON pipeline_phases.id = applications.phase_id
		WHERE applications.job_id = $1`, id), &phase.ID, &phase.Name, &phase.Position, &phase.IsClosed, &phase.FollowUpDays)
	switch {
	case err == nil:
		details.Application = &application
		details.Phase = &phase
	case !errors.Is(err, ErrApplicationNotFound):
		return JobDetails{}, err
	}

	var connectionsErr error
	switch {
	case job.CompanyID != nil:
		details.Connections, connectionsErr = s.ListCompanyConnections(ctx, *job.CompanyID)
	case details.CompanyName != nil:
		details.Connections, connectionsErr = s.ListConnectionsAtCompanyName(ctx, *details.CompanyName)
	default:
		details.Connections = []Connection{}
	}
	return details, connectionsErr
}

// labelJobFacts flattens the facts (see FlattenJobFacts) and orders them as
// the schema does (see listJobFactLabels), then any fact the schema doesn't
// name, labelling each with the schema's title and description and its
// evidence when it was read with some.
func labelJobFacts(rawFacts, schema json.RawMessage) ([]JobFactEntry, error) {
	values, evidence, err := FlattenJobFacts(rawFacts)
	if err != nil {
		return nil, err
	}
	labels, err := listJobFactLabels(schema)
	if err != nil {
		return nil, err
	}
	labelled := map[string]bool{}
	for _, label := range labels {
		labelled[label.Key] = true
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if !labelled[key] {
			labels = append(labels, jobFactLabel{Key: key, Title: key})
		}
	}

	entries := make([]JobFactEntry, 0, len(values))
	for _, label := range labels {
		value, hasFact := values[label.Key]
		if !hasFact {
			continue
		}
		entries = append(entries, JobFactEntry{Key: label.Key, Title: label.Title, Description: label.Description, Value: value, Evidence: evidence[label.Key]})
	}
	return entries, nil
}

// JobFactColumn is one fact the current job_facts prompt reads, as a column
// of the jobs list: its key and the schema's title for it.
type JobFactColumn struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

// ListJobFactColumns returns the facts the current job_facts prompt reads, in
// the schema's order (see listJobFactLabels).
func (s *Store) ListJobFactColumns(ctx context.Context) ([]JobFactColumn, error) {
	prompt, err := s.GetLatestAgentPrompt(ctx, AgentPromptKindJobFacts)
	if errors.Is(err, ErrAgentPromptNotFound) || (err == nil && len(prompt.ResultSchema) == 0) {
		return []JobFactColumn{}, nil
	}
	if err != nil {
		return nil, err
	}
	labels, err := listJobFactLabels(prompt.ResultSchema)
	if err != nil {
		return nil, err
	}
	columns := []JobFactColumn{}
	for _, label := range labels {
		columns = append(columns, JobFactColumn{Key: label.Key, Title: label.Title})
	}
	return columns, nil
}
