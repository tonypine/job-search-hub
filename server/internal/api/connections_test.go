package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const madeUpConnections = "Notes:\n\"About missing emails.\"\n\nFirst Name,Last Name,URL,Email Address,Company,Position,Connected On\n" +
	"Ada,Lovelace,https://www.linkedin.com/in/ada-example,,\"Acme, Inc.\",Engineering Manager,28 Sep 2026\n" +
	"Grace,Hopper,https://www.linkedin.com/in/grace-example,,Globex,Engineer,03 Jan 2019\n"

func TestConnectionsAreImportedOnceAndTiedToKnownCompanies(t *testing.T) {
	service := startAPI(t)
	acme, _, err := service.hub.CreateCompany(context.Background(), store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodPost, service.url+"/v1/connections/import", ownerToken, madeUpConnections)
	var first struct {
		Added, Updated, Matched, Skipped int
	}
	if json.Unmarshal(body, &first); status != http.StatusOK || first.Added != 2 || first.Matched != 1 {
		t.Fatalf("first import: %d %s", status, body)
	}
	status, body = send(t, http.MethodPost, service.url+"/v1/connections/import", ownerToken, madeUpConnections)
	var second struct{ Added, Updated int }
	if json.Unmarshal(body, &second); status != http.StatusOK || second.Added != 0 || second.Updated != 2 {
		t.Fatalf("second import: %d %s; want the same two updated", status, body)
	}

	atAcme, _ := service.hub.ListCompanyConnections(context.Background(), acme.ID)
	if len(atAcme) != 1 || atAcme[0].FirstName != "Ada" {
		t.Fatalf("connections at Acme = %+v", atAcme)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/connections", ownerToken, "")
	var summary store.ConnectionsSummary
	if json.Unmarshal(body, &summary); status != http.StatusOK || summary.Count != 2 || summary.Matched != 1 || summary.LastImportedAt == nil {
		t.Fatalf("summary: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/connections/import", ownerToken, "Name,Email\n"); status != http.StatusBadRequest {
		t.Fatalf("another file: %d", status)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/connections", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("without the owner token: %d", status)
	}
}
