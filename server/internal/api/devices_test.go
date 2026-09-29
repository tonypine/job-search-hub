package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAPairedPhoneActsAsTheOwnerUntilItIsRevoked(t *testing.T) {
	service := startAPI(t)

	status, body := send(t, http.MethodPost, service.url+"/v1/devices", ownerToken, `{"name":"Sam's phone"}`)
	var paired struct {
		Device struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"device"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &paired); status != http.StatusCreated || err != nil || !strings.HasPrefix(paired.Token, "hubdev_") || paired.Device.Name != "Sam's phone" {
		t.Fatalf("pair: %d %s", status, body)
	}

	if status, _ := send(t, http.MethodGet, service.url+"/v1/jobs", paired.Token, ""); status != http.StatusOK {
		t.Fatalf("the phone reading jobs: %d", status)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/devices", paired.Token, `{"name":"Another"}`); status != http.StatusForbidden {
		t.Errorf("a phone pairing another: %d, want 403", status)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/devices", paired.Token, "")
	if status != http.StatusOK || strings.Contains(string(body), paired.Token) || !strings.Contains(string(body), `"last_seen_at"`) {
		t.Fatalf("list: %d %s; want the device seen and its token never shown again", status, body)
	}

	if status, _ := send(t, http.MethodDelete, service.url+"/v1/devices/"+paired.Device.ID, ownerToken, ""); status != http.StatusOK {
		t.Fatalf("revoke: %d", status)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/jobs", paired.Token, ""); status != http.StatusUnauthorized {
		t.Errorf("a revoked phone: %d, want 401", status)
	}
}
