// Package tokens verifies the bearer tokens the hub accepts, and turns a
// verified token into the actor its writes are attributed to.
package tokens

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const ScopeOwner = "owner"

// NewVerifier accepts the owner token.
func NewVerifier(ownerToken string) auth.TokenVerifier {
	return func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if subtle.ConstantTimeCompare([]byte(token), []byte(ownerToken)) == 1 {
			return &auth.TokenInfo{UserID: ScopeOwner, Scopes: []string{ScopeOwner}}, nil
		}
		return nil, auth.ErrInvalidToken
	}
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
	return store.Actor{}, errors.New("the token has no scope the hub recognizes")
}
