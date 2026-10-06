package tokens_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

var ownerToken = strings.Repeat("o", 64)

type fakeAgentRuns struct {
	running map[string]store.AgentRun
	devices map[string]store.Device
	// appVersions are the versions the devices were seen with.
	appVersions *[]string
}

func (runs fakeAgentRuns) GetActiveDeviceByTokenHash(_ context.Context, tokenHash []byte, appVersion string) (store.Device, error) {
	for token, device := range runs.devices {
		if bytes.Equal(tokens.HashToken(token), tokenHash) {
			*runs.appVersions = append(*runs.appVersions, appVersion)
			return device, nil
		}
	}
	return store.Device{}, store.ErrDeviceNotFound
}

func (runs fakeAgentRuns) GetRunningAgentRunByTokenHash(_ context.Context, tokenHash []byte) (store.AgentRun, error) {
	for token, run := range runs.running {
		if bytes.Equal(tokens.HashToken(token), tokenHash) {
			return run, nil
		}
	}
	return store.AgentRun{}, store.ErrAgentRunNotFound
}

func TestVerifierAcceptsTheOwnerToken(t *testing.T) {
	info, err := tokens.NewVerifier(ownerToken, fakeAgentRuns{})(context.Background(), ownerToken, nil)
	if err != nil || !slices.Equal(info.Scopes, []string{tokens.ScopeOwner}) {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestVerifierAcceptsARunningAgentRunsToken(t *testing.T) {
	token, _ := tokens.NewAgentRunToken()
	run := store.AgentRun{ID: uuid.New(), TokenExpiresAt: time.Now().Add(time.Hour)}
	verify := tokens.NewVerifier(ownerToken, fakeAgentRuns{running: map[string]store.AgentRun{token: run}})

	info, err := verify(context.Background(), token, nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !slices.Equal(info.Scopes, []string{tokens.ScopeAgentRun}) || !info.Expiration.Equal(run.TokenExpiresAt) || info.Extra["agent_run_id"] != run.ID {
		t.Fatalf("info = %+v", info)
	}
}

func TestVerifierNotesTheAppVersionAPairedPhoneCallsWith(t *testing.T) {
	token, _ := tokens.NewDeviceToken()
	device := store.Device{ID: uuid.New()}
	var seen []string
	verify := tokens.NewVerifier(ownerToken, fakeAgentRuns{devices: map[string]store.Device{token: device}, appVersions: &seen})

	request := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	request.Header.Set("X-Hub-Client", "android/0.1.252")
	info, err := verify(context.Background(), token, request)
	if err != nil || info.Extra["device_id"] != device.ID {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	if _, err := verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, []string{"0.1.252", ""}) {
		t.Fatalf("app versions = %q, want the header's, then none", seen)
	}
}

func TestVerifierRejectsAnyOtherToken(t *testing.T) {
	verify := tokens.NewVerifier(ownerToken, fakeAgentRuns{})
	for _, token := range []string{"", strings.Repeat("x", 64), ownerToken[:63]} {
		if _, err := verify(context.Background(), token, nil); !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("token %q: err = %v, want ErrInvalidToken", token, err)
		}
	}
}

func TestNewAgentRunTokenReturnsItsHash(t *testing.T) {
	token, hash := tokens.NewAgentRunToken()
	other, _ := tokens.NewAgentRunToken()
	if token == other || len(token) < 26 || !bytes.Equal(hash, tokens.HashToken(token)) {
		t.Fatalf("token=%q other=%q", token, other)
	}
}

func TestGetActorRequiresAVerifiedToken(t *testing.T) {
	if _, err := tokens.GetActor(context.Background()); err == nil {
		t.Fatal("expected an error without a verified token")
	}
}
