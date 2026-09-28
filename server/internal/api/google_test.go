package api_test

import (
	"net/http"
	"strings"
	"testing"
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
