package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrAgentPromptNotFound = errors.New("agent prompt not found")

// AgentPrompt is one version of an agent's instructions. Versions are never
// edited; saving a prompt adds the next version, and the highest is active.
type AgentPrompt struct {
	ID        uuid.UUID `json:"id"`
	Kind      string    `json:"kind"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

const agentPromptColumns = `id, kind, version, body, note, created_at`

func scanAgentPrompt(row pgx.Row) (AgentPrompt, error) {
	var prompt AgentPrompt
	err := row.Scan(&prompt.ID, &prompt.Kind, &prompt.Version, &prompt.Body, &prompt.Note, &prompt.CreatedAt)
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

// SaveAgentPrompt adds body as the next version of the kind's prompt.
func (s *Store) SaveAgentPrompt(ctx context.Context, actor Actor, kind, body, note string) (AgentPrompt, error) {
	if kind != AgentRunKindCompanyTriage {
		return AgentPrompt{}, ErrAgentPromptNotFound
	}
	if strings.TrimSpace(body) == "" {
		return AgentPrompt{}, errors.New("a prompt needs a body")
	}

	var prompt AgentPrompt
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		prompt, err = scanAgentPrompt(tx.QueryRow(ctx, `
			INSERT INTO agent_prompts (kind, version, body, note)
			SELECT $1, COALESCE(MAX(version), 0) + 1, $2, $3 FROM agent_prompts WHERE kind = $1
			RETURNING `+agentPromptColumns, kind, body, note))
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
