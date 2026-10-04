package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
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
	// AgentPromptKindProfileInterview starts the Claude session that
	// interviews the owner to deepen their knowledge base.
	AgentPromptKindProfileInterview = "profile_interview"
	// AgentPromptKindJobBrief writes a job's brief: how well the owner
	// matches it, from the knowledge base.
	AgentPromptKindJobBrief = "job_brief"
	// AgentPromptKindJobCV tailors the base CV to a pursued job.
	AgentPromptKindJobCV = "job_cv"
	// AgentPromptKindRecruiterScreen reads a pursued job's tailored CV as
	// the job's recruiter would, listing the likely reasons to reject it.
	AgentPromptKindRecruiterScreen = "recruiter_screen"
	// AgentPromptKindMarketGaps plans how to close the skills good fits keep
	// asking for and the knowledge base lacks.
	AgentPromptKindMarketGaps = "market_gaps"
	// AgentPromptKindInterviewPrep writes a pursued job's interview pack:
	// the likely questions and the confirmed cases to tell.
	AgentPromptKindInterviewPrep = "interview_prep"
	// AgentPromptKindHiringThread reads a comment of Hacker News' monthly
	// "Who is hiring?" thread into its company and roles.
	AgentPromptKindHiringThread = "hiring_thread"
)

// AgentPromptKinds are every prompt the hub keeps, the same kinds the
// agent_prompts table's check allows.
var AgentPromptKinds = []string{
	AgentRunKindCompanyTriage, AgentPromptKindJobFacts, AgentPromptKindCompanySession, AgentPromptKindJobSession,
	AgentPromptKindOutreachDraft, AgentPromptKindMailTriage, AgentPromptKindLinkedInConversation,
	AgentPromptKindRecruiterReply, AgentPromptKindProfileAudit, AgentRunKindJobFinder, AgentRunKindProfileSeed, AgentPromptKindProfileInterview,
	AgentPromptKindJobBrief, AgentPromptKindJobCV, AgentRunKindJobFix, AgentPromptKindRecruiterScreen,
	AgentPromptKindMarketGaps, AgentPromptKindInterviewPrep, AgentPromptKindHiringThread,
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
	// Examples are shown to the model before the real input, as earlier
	// turns of the conversation.
	Examples  []chatcompletions.Example `json:"examples,omitempty"`
	Note      string                    `json:"note,omitempty"`
	CreatedAt time.Time                 `json:"created_at"`
}

// NewAgentPrompt is the next version of a prompt. A nil ResultSchema keeps
// the previous version's schema, and nil Examples its examples; an empty
// Examples removes them.
type NewAgentPrompt struct {
	Kind         string
	Body         string
	ResultSchema json.RawMessage
	Examples     []chatcompletions.Example
	Note         string
}

const agentPromptColumns = `id, kind, version, body, result_schema, examples, note, created_at`

func scanAgentPrompt(row pgx.Row) (AgentPrompt, error) {
	var prompt AgentPrompt
	err := row.Scan(&prompt.ID, &prompt.Kind, &prompt.Version, &prompt.Body, &prompt.ResultSchema, &prompt.Examples, &prompt.Note, &prompt.CreatedAt)
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
	if !slices.Contains(AgentPromptKinds, input.Kind) {
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

	var examples *string
	if input.Examples != nil {
		encoded, err := json.Marshal(input.Examples)
		if err != nil {
			return AgentPrompt{}, err
		}
		text := string(encoded)
		examples = &text
	}

	var prompt AgentPrompt
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		prompt, err = scanAgentPrompt(tx.QueryRow(ctx, `
			INSERT INTO agent_prompts (kind, version, body, result_schema, examples, note)
			SELECT $1, COALESCE(MAX(version), 0) + 1, $2,
			       COALESCE($3::json, (SELECT result_schema FROM agent_prompts WHERE kind = $1 ORDER BY version DESC LIMIT 1)),
			       COALESCE($4::jsonb, (SELECT examples FROM agent_prompts WHERE kind = $1 ORDER BY version DESC LIMIT 1), '[]'::jsonb), $5
			FROM agent_prompts WHERE kind = $1
			RETURNING `+agentPromptColumns, input.Kind, input.Body, input.ResultSchema, examples, input.Note))
		if err != nil {
			return err
		}
		if err := checkExamples(prompt.Examples, prompt.ResultSchema); err != nil {
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
	for _, kind := range slices.Sorted(slices.Values(AgentPromptKinds)) {
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
// checkExamples refuses an example without an input, or whose answer isn't
// a JSON object matching the schema, when the prompt has one.
func checkExamples(examples []chatcompletions.Example, schemaText json.RawMessage) error {
	var resolved *jsonschema.Resolved
	if len(schemaText) > 0 {
		var schema jsonschema.Schema
		if err := json.Unmarshal(schemaText, &schema); err != nil {
			return fmt.Errorf("read the result schema: %w", err)
		}
		var err error
		if resolved, err = schema.Resolve(nil); err != nil {
			return fmt.Errorf("resolve the result schema: %w", err)
		}
	}
	for index, example := range examples {
		if strings.TrimSpace(example.Input) == "" {
			return fmt.Errorf("example %d has no input", index+1)
		}
		var answer map[string]any
		if err := json.Unmarshal(example.Answer, &answer); err != nil {
			return fmt.Errorf("example %d's answer is not a JSON object", index+1)
		}
		if resolved != nil {
			if err := resolved.Validate(answer); err != nil {
				return fmt.Errorf("example %d's answer doesn't match the result schema: %w", index+1, err)
			}
		}
	}
	return nil
}

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
