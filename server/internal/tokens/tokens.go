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
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/hubclients"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	ScopeOwner    = "owner"
	ScopeAgentRun = "agent_run"
)

// DevicePrefix starts every device token, so the verifier knows where to
// look one up.
const DevicePrefix = "hubdev_"

type tokenLookup interface {
	GetRunningAgentRunByTokenHash(ctx context.Context, tokenHash []byte) (store.AgentRun, error)
	GetActiveDeviceByTokenHash(ctx context.Context, tokenHash []byte, appVersion string) (store.Device, error)
}

// NewVerifier accepts the owner token; a paired device's token, which acts
// as the owner, noting the app version its X-Hub-Client header names; and the token of any agent run that is still running and
// unexpired.
func NewVerifier(ownerToken string, lookup tokenLookup) auth.TokenVerifier {
	return func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		if subtle.ConstantTimeCompare([]byte(token), []byte(ownerToken)) == 1 {
			return &auth.TokenInfo{UserID: ScopeOwner, Scopes: []string{ScopeOwner}}, nil
		}
		if strings.HasPrefix(token, DevicePrefix) {
			appVersion := ""
			if r != nil {
				if client, found := hubclients.FromRequest(r); found {
					appVersion = client.Version
				}
			}
			device, err := lookup.GetActiveDeviceByTokenHash(ctx, HashToken(token), appVersion)
			if errors.Is(err, store.ErrDeviceNotFound) {
				return nil, auth.ErrInvalidToken
			}
			if err != nil {
				return nil, err
			}
			return &auth.TokenInfo{UserID: "device:" + device.ID.String(), Scopes: []string{ScopeOwner}, Extra: map[string]any{"device_id": device.ID}}, nil
		}

		run, err := lookup.GetRunningAgentRunByTokenHash(ctx, HashToken(token))
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

// NewDeviceToken returns a fresh device token and the hash the store keeps
// in its place.
func NewDeviceToken() (string, []byte) {
	token := DevicePrefix + rand.Text()
	return token, HashToken(token)
}

// IsOwnersOwnToken says whether the request carries the owner token itself,
// rather than a device's: pairing phones is done from the Mac.
func IsOwnersOwnToken(ctx context.Context) bool {
	info := auth.TokenInfoFromContext(ctx)
	return info != nil && info.UserID == ScopeOwner
}

// GetDeviceID returns the paired device the request's token belongs to.
func GetDeviceID(ctx context.Context) (uuid.UUID, bool) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return uuid.UUID{}, false
	}
	id, found := info.Extra["device_id"].(uuid.UUID)
	return id, found
}

// NewAgentRunToken returns a fresh random token and the hash the store keeps
// in its place.
func NewAgentRunToken() (string, []byte) {
	token := rand.Text()
	return token, HashToken(token)
}

func HashToken(token string) []byte {
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
