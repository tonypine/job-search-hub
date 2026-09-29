package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrAgentPromptNotFound = errors.New("agent prompt not found")

// AgentPromptKindJobFacts is the prompt that reads a job's facts from its
// description; its ResultSchema lists the facts.
const AgentPromptKindJobFacts = "job_facts"

// AgentPromptKindCompanySession and AgentPromptKindJobSession start the Claude
// sessions the owner opens on a company or a job.
const (
	AgentPromptKindCompanySession = "company_session"
	AgentPromptKindJobSession     = "job_session"
	// AgentPromptKindOutreachDraft is the request a session gets when the
	// owner asks it to draft outreach.
	AgentPromptKindOutreachDraft = "outreach_draft"
	// AgentPromptKindMailTriage sorts the received mail no rule could sort.
	AgentPromptKindMailTriage = "mail_triage"
	// AgentPromptKindLinkedInConversation sorts the LinkedIn conversations
	// others started, finding the recruiters.
	AgentPromptKindLinkedInConversation = "linkedin_conversation"
	// AgentPromptKindRecruiterReply drafts a message back to a recruiter
	// who wrote to the owner before.
	AgentPromptKindRecruiterReply = "recruiter_reply"
	// AgentPromptKindProfileAudit audits the owner's LinkedIn profile for the
	// recruiters who search for the roles they want.
	AgentPromptKindProfileAudit = "profile_audit"
)

var agentPromptKinds = map[string]bool{
	AgentRunKindCompanyTriage: true, AgentPromptKindJobFacts: true, AgentPromptKindCompanySession: true, AgentPromptKindJobSession: true,
	AgentPromptKindOutreachDraft: true, AgentPromptKindMailTriage: true, AgentPromptKindLinkedInConversation: true,
	AgentPromptKindRecruiterReply: true, AgentPromptKindProfileAudit: true, AgentRunKindJobFinder: true, AgentRunKindProfileSeed: true,
}

// AgentPrompt is one version of an agent's instructions, and of the JSON
// schema its answer must match when the prompt carries one. Versions are
// never edited; saving a prompt adds the next version, and the highest is
// active.
type AgentPrompt struct {
	ID           uuid.UUID       `json:"id"`
	Kind         string          `json:"kind"`
	Version      int             `json:"version"`
	Body         string          `json:"body"`
	ResultSchema json.RawMessage `json:"result_schema,omitempty"`
	Note         string          `json:"note,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// NewAgentPrompt is the next version of a prompt. A nil ResultSchema keeps
// the previous version's schema.
type NewAgentPrompt struct {
	Kind         string
	Body         string
	ResultSchema json.RawMessage
	Note         string
}

const agentPromptColumns = `id, kind, version, body, result_schema, note, created_at`

func scanAgentPrompt(row pgx.Row) (AgentPrompt, error) {
	var prompt AgentPrompt
	err := row.Scan(&prompt.ID, &prompt.Kind, &prompt.Version, &prompt.Body, &prompt.ResultSchema, &prompt.Note, &prompt.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentPrompt{}, ErrAgentPromptNotFound
	}
	return prompt, err
}

func (s *Store) GetLatestAgentPrompt(ctx context.Context, kind string) (AgentPrompt, error) {
	return scanAgentPrompt(s.pool.QueryRow(ctx, `
		SELECT `+agentPromptColumns+` FROM agent_prompts WHERE kind = $1 ORDER BY version DESC LIMIT 1`, kind))
}

func (s *Store) GetAgentPrompt(ctx context.Context, kind string, version int) (AgentPrompt, error) {
	return scanAgentPrompt(s.pool.QueryRow(ctx, `
		SELECT `+agentPromptColumns+` FROM agent_prompts WHERE kind = $1 AND version = $2`, kind, version))
}

// ListAgentPromptVersions returns every version of the kind's prompt, the
// newest first.
func (s *Store) ListAgentPromptVersions(ctx context.Context, kind string) ([]AgentPrompt, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+agentPromptColumns+` FROM agent_prompts WHERE kind = $1 ORDER BY version DESC`, kind)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AgentPrompt, error) { return scanAgentPrompt(row) })
}

// SaveAgentPrompt adds the next version of the kind's prompt.
func (s *Store) SaveAgentPrompt(ctx context.Context, actor Actor, input NewAgentPrompt) (AgentPrompt, error) {
	if !agentPromptKinds[input.Kind] {
		return AgentPrompt{}, ErrAgentPromptNotFound
	}
	if strings.TrimSpace(input.Body) == "" {
		return AgentPrompt{}, errors.New("a prompt needs a body")
	}
	if input.ResultSchema != nil {
		if err := checkResultSchema(input.ResultSchema); err != nil {
			return AgentPrompt{}, err
		}
	}

	var prompt AgentPrompt
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		prompt, err = scanAgentPrompt(tx.QueryRow(ctx, `
			INSERT INTO agent_prompts (kind, version, body, result_schema, note)
			SELECT $1, COALESCE(MAX(version), 0) + 1, $2,
			       COALESCE($3::jsonb, (SELECT result_schema FROM agent_prompts WHERE kind = $1 ORDER BY version DESC LIMIT 1)), $4
			FROM agent_prompts WHERE kind = $1
			RETURNING `+agentPromptColumns, input.Kind, input.Body, input.ResultSchema, input.Note))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "agent_prompt", entityID: prompt.ID, operation: "create",
			after: map[string]any{"kind": prompt.Kind, "version": prompt.Version, "note": prompt.Note},
		})
	})
	return prompt, err
}

// SeedAgentPrompts saves, as the first version of each kind that has none,
// the prompt in <kind>.md, with <kind>.schema.json as its result schema when
// there is one. The prompts live in the database; the seed is kept out of the
// repo. It returns the kinds it saved.
func (s *Store) SeedAgentPrompts(ctx context.Context, seed fs.FS) ([]string, error) {
	var seeded []string
	for _, kind := range slices.Sorted(maps.Keys(agentPromptKinds)) {
		body, err := fs.ReadFile(seed, kind+".md")
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return seeded, err
		}
		var schema json.RawMessage
		if schema, err = fs.ReadFile(seed, kind+".schema.json"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return seeded, err
		}
		if _, err := s.GetLatestAgentPrompt(ctx, kind); !errors.Is(err, ErrAgentPromptNotFound) {
			if err != nil {
				return seeded, err
			}
			continue
		}
		if _, err := s.SaveAgentPrompt(ctx, Actor{Kind: ActorSystem}, NewAgentPrompt{
			Kind: kind, Body: string(body), ResultSchema: schema, Note: "Seeded from " + kind + ".md",
		}); err != nil {
			return seeded, fmt.Errorf("seed the %s prompt: %w", kind, err)
		}
		seeded = append(seeded, kind)
	}
	return seeded, nil
}

// checkResultSchema refuses a schema that is not a JSON object, or that uses
// a list of types anywhere: LM Studio's MLX backend rejects "type": [...], and
// anyOf says the same thing in a form it accepts.
func checkResultSchema(schema json.RawMessage) error {
	var decoded any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return errors.New("the result schema is not JSON: " + err.Error())
	}
	root, isObject := decoded.(map[string]any)
	if !isObject {
		return errors.New("the result schema must be a JSON object")
	}
	if hasTypeList(root) {
		return errors.New(`the result schema lists several types under "type"; use anyOf, e.g. {"anyOf":[{"type":"integer"},{"type":"null"}]}`)
	}
	return nil
}

func hasTypeList(node any) bool {
	switch value := node.(type) {
	case map[string]any:
		if _, isList := value["type"].([]any); isList {
			return true
		}
		for _, child := range value {
			if hasTypeList(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if hasTypeList(child) {
				return true
			}
		}
	}
	return false
}
