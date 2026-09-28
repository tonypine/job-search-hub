package tokens_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

var ownerToken = strings.Repeat("o", 64)

func TestVerifierAcceptsOnlyTheOwnerToken(t *testing.T) {
	verify := tokens.NewVerifier(ownerToken)

	info, err := verify(context.Background(), ownerToken, nil)
	if err != nil || info.UserID != tokens.ScopeOwner {
		t.Fatalf("owner token: info=%+v err=%v", info, err)
	}
	for _, token := range []string{"", strings.Repeat("x", 64), ownerToken[:63]} {
		if _, err := verify(context.Background(), token, nil); !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("token %q: err = %v, want ErrInvalidToken", token, err)
		}
	}
}

func TestGetActorRequiresAVerifiedToken(t *testing.T) {
	if _, err := tokens.GetActor(context.Background()); err == nil {
		t.Fatal("expected an error without a verified token")
	}
}
