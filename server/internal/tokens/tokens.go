// Package tokens verifies the bearer tokens the hub accepts, and turns a
// verified token into the actor its writes are attributed to.
package tokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	ScopeOwner    = "owner"
	ScopeAgentRun = "agent_run"
)

type agentRunLookup interface {
	GetRunningAgentRunByTokenHash(ctx context.Context, tokenHash []byte) (store.AgentRun, error)
}

// NewVerifier accepts the owner token, and the token of any agent run that is
// still running and unexpired.
func NewVerifier(ownerToken string, agentRuns agentRunLookup) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if subtle.ConstantTimeCompare([]byte(token), []byte(ownerToken)) == 1 {
			return &auth.TokenInfo{UserID: ScopeOwner, Scopes: []string{ScopeOwner}}, nil
		}

		run, err := agentRuns.GetRunningAgentRunByTokenHash(ctx, HashAgentRunToken(token))
		if errors.Is(err, store.ErrAgentRunNotFound) {
			return nil, auth.ErrInvalidToken
		}
		if err != nil {
			return nil, err
		}
		return &auth.TokenInfo{
			UserID:     "agent-run:" + run.ID.String(),
			Scopes:     []string{ScopeAgentRun},
			Expiration: run.TokenExpiresAt,
			Extra:      map[string]any{"agent_run_id": run.ID},
		}, nil
	}
}

// NewAgentRunToken returns a fresh random token and the hash the store keeps
// in its place.
func NewAgentRunToken() (string, []byte) {
	token := rand.Text()
	return token, HashAgentRunToken(token)
}

func HashAgentRunToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

// GetActor returns who the verified token in ctx belongs to.
func GetActor(ctx context.Context) (store.Actor, error) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return store.Actor{}, errors.New("the request carries no verified token")
	}
	if slices.Contains(info.Scopes, ScopeOwner) {
		return store.Actor{Kind: store.ActorOwner}, nil
	}
	if agentRunID, ok := info.Extra["agent_run_id"].(uuid.UUID); ok && slices.Contains(info.Scopes, ScopeAgentRun) {
		return store.Actor{Kind: store.ActorAgentRun, AgentRunID: agentRunID}, nil
	}
	return store.Actor{}, errors.New("the token has no scope the hub recognizes")
}
