package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestUpdatesAreRecordedListedAndMarkedSeenOverREST(t *testing.T) {
	service := startAPI(t)
	company, _, _ := service.hub.CreateCompany(context.Background(), store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})

	status, body := send(t, http.MethodPost, service.url+"/v1/updates", ownerToken, `{"kind":"reply","title":"Acme replied","company_id":"`+company.ID.String()+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("record: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/updates?unseen=true", ownerToken, "")
	var list store.UpdateList
	if err := json.Unmarshal(body, &list); status != http.StatusOK || err != nil || list.UnseenCount != 1 || *list.Updates[0].CompanyName != "Acme" {
		t.Fatalf("list: %d %s", status, body)
	}
	status, body = send(t, http.MethodPost, service.url+"/v1/updates/seen", ownerToken, `{"company_id":"`+company.ID.String()+`"}`)
	if status != http.StatusOK || string(body) != "{\"marked\":1}\n" {
		t.Fatalf("seen: %d %q", status, body)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/updates", ownerToken, `{"kind":"reply"}`); status != http.StatusBadRequest {
		t.Errorf("an update without a title: %d, want 400", status)
	}
}
