package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheProfileRoundTripsAndRecordsTheSave(t *testing.T) {
	service := startAPI(t)

	status, body := send(t, http.MethodPut, service.url+"/v1/profile", ownerToken, `{"body":"# Candidate\nSenior engineer."}`)
	if status != http.StatusOK {
		t.Fatalf("put: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/profile", ownerToken, "")
	var profile store.OwnerProfile
	if err := json.Unmarshal(body, &profile); status != http.StatusOK || err != nil || profile.Body != "# Candidate\nSenior engineer." {
		t.Fatalf("get: %d %s", status, body)
	}

	var saves int
	err := service.pool.QueryRow(context.Background(), `SELECT count(*) FROM changes WHERE entity_type = 'owner_profile' AND actor_kind = 'owner'`).Scan(&saves)
	if err != nil || saves != 1 {
		t.Fatalf("profile changes = %d, err = %v, want 1", saves, err)
	}
}
