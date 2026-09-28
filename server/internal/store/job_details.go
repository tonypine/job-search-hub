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
// it, and the value read.
type JobFactEntry struct {
	Key         string          `json:"key"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Value       json.RawMessage `json:"value"`
}

func (s *Store) GetJobDetails(ctx context.Context, id uuid.UUID) (JobDetails, error) {
	var details JobDetails
	job, err := scanJob(s.pool.QueryRow(ctx, `
		SELECT `+prefixedJobColumns+`, companies.name
		FROM jobs LEFT JOIN companies ON companies.id = jobs.company_id
		WHERE jobs.id = $1`, id), &details.CompanyName)
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
	case !errors.Is(err, pgx.ErrNoRows):
		return JobDetails{}, err
	}

	var phase PipelinePhase
	application, err := scanApplication(s.pool.QueryRow(ctx, `
		SELECT applications.id, applications.job_id, applications.company_id, applications.phase_id, applications.closed_reason,
		       applications.notes, applications.phase_entered_at, applications.created_at, applications.updated_at,
		       pipeline_phases.id, pipeline_phases.name, pipeline_phases.position, pipeline_phases.is_closed
		FROM applications JOIN pipeline_phases ON pipeline_phases.id = applications.phase_id
		WHERE applications.job_id = $1`, id), &phase.ID, &phase.Name, &phase.Position, &phase.IsClosed)
	switch {
	case err == nil:
		details.Application = &application
		details.Phase = &phase
	case !errors.Is(err, ErrApplicationNotFound):
		return JobDetails{}, err
	}
	return details, nil
}

// labelJobFacts orders the facts as the schema's required list does, then
// the schema's other properties, then any fact the schema does not name, and
// labels each with the schema's title and description. A fact the schema
// does not title is labelled with its key.
func labelJobFacts(rawFacts, schema json.RawMessage) ([]JobFactEntry, error) {
	var facts map[string]json.RawMessage
	if err := json.Unmarshal(rawFacts, &facts); err != nil {
		return nil, err
	}
	var parsedSchema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if len(schema) > 0 {
		if err := json.Unmarshal(schema, &parsedSchema); err != nil {
			return nil, err
		}
	}

	var orderedKeys []string
	seen := map[string]bool{}
	addKeys := func(keys []string) {
		for _, key := range keys {
			if _, hasFact := facts[key]; hasFact && !seen[key] {
				seen[key] = true
				orderedKeys = append(orderedKeys, key)
			}
		}
	}
	addKeys(parsedSchema.Required)
	addKeys(slices.Sorted(maps.Keys(parsedSchema.Properties)))
	addKeys(slices.Sorted(maps.Keys(facts)))

	entries := make([]JobFactEntry, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		property := parsedSchema.Properties[key]
		title := property.Title
		if title == "" {
			title = key
		}
		entries = append(entries, JobFactEntry{Key: key, Title: title, Description: property.Description, Value: facts[key]})
	}
	return entries, nil
}
