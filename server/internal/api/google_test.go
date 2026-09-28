package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestAHubWithoutAGoogleClientSaysSo(t *testing.T) {
	service := startAPI(t)

	if status, body := send(t, http.MethodGet, service.url+"/v1/google", ownerToken, ""); status != http.StatusOK || !strings.Contains(string(body), `"configured":false`) {
		t.Fatalf("status: %d %s", status, body)
	}
	if status, body := send(t, http.MethodPost, service.url+"/v1/google/sign-in", ownerToken, ""); status != http.StatusServiceUnavailable || !strings.Contains(string(body), "HUB_GOOGLE_OAUTH_CLIENT_FILE") {
		t.Fatalf("sign-in: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/google/callback?state=x&code=y", "", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("callback: %d", status)
	}
}

func TestTheStatusNamesWhatAnOlderSignInDidntGrant(t *testing.T) {
	service := startAPI(t)
	olderScopes := []string{"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/calendar.readonly"}
	if _, err := service.hub.SaveGoogleConnection(context.Background(), store.Actor{Kind: store.ActorOwner}, "owner@example.com", "refresh-1", olderScopes); err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/google", ownerToken, "")
	if status != http.StatusOK || !strings.Contains(string(body), `"missing_scopes":["https://www.googleapis.com/auth/pubsub"]`) {
		t.Fatalf("status: %d %s", status, body)
	}
}
