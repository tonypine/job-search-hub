package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestProfileEntriesRoundTripAndKeepCasesWithTheirRole(t *testing.T) {
	service := startAPI(t)
	post := func(body string) (int, store.ProfileEntry) {
		status, raw := send(t, http.MethodPost, service.url+"/v1/profile/entries", ownerToken, body)
		var entry store.ProfileEntry
		json.Unmarshal(raw, &entry)
		return status, entry
	}

	status, role := post(`{"kind":"role","title":"Senior Software Engineer","organization":"Acme","start_month":"2022-05","end_month":"2024-01","source":"cv","source_detail":"CV page 1"}`)
	if status != http.StatusCreated || role.ConfirmedAt != nil {
		t.Fatalf("role: %d %+v; want a new, unconfirmed entry", status, role)
	}
	status, entryCase := post(`{"kind":"case","role_id":"` + role.ID.String() + `","title":"Moved checkout to one page","skills":["TypeScript","React","typescript"],"outcome":"Checkout time halved","source":"interview"}`)
	if status != http.StatusCreated || entryCase.RoleID == nil || *entryCase.RoleID != role.ID || len(entryCase.Skills) != 2 {
		t.Fatalf("case: %d %+v; want it tied to the role, skills deduplicated", status, entryCase)
	}
	if status, _ := post(`{"kind":"case","role_id":"` + entryCase.ID.String() + `","title":"Nested","source":"owner"}`); status != http.StatusBadRequest {
		t.Errorf("a case under a case: %d, want 400", status)
	}
	if status, _ := post(`{"kind":"role","title":"Engineer","start_month":"May 2022","source":"cv"}`); status != http.StatusBadRequest {
		t.Errorf("a month in words: %d, want 400", status)
	}

	status, raw := send(t, http.MethodPost, service.url+"/v1/profile/entries/confirm", ownerToken, `{"ids":["`+role.ID.String()+`"]}`)
	if status != http.StatusOK {
		t.Fatalf("confirm: %d %s", status, raw)
	}
	status, raw = send(t, http.MethodGet, service.url+"/v1/profile/entries?confirmed=true", ownerToken, "")
	var confirmed struct {
		Entries []store.ProfileEntry `json:"entries"`
	}
	if json.Unmarshal(raw, &confirmed); status != http.StatusOK || len(confirmed.Entries) != 1 || confirmed.Entries[0].ID != role.ID {
		t.Fatalf("confirmed list: %d %s", status, raw)
	}

	status, raw = send(t, http.MethodPut, service.url+"/v1/profile/entries/"+role.ID.String(), ownerToken,
		`{"kind":"role","title":"Senior Software Engineer","organization":"Acme Inc.","start_month":"2022-05","end_month":"2024-01","source":"cv"}`)
	var edited store.ProfileEntry
	if json.Unmarshal(raw, &edited); status != http.StatusOK || edited.Organization != "Acme Inc." || edited.ConfirmedAt == nil {
		t.Fatalf("the owner's edit: %d %s; want it saved and still confirmed", status, raw)
	}

	if status, _ := send(t, http.MethodDelete, service.url+"/v1/profile/entries/"+role.ID.String(), ownerToken, ""); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	cases, err := service.hub.ListProfileEntries(context.Background(), store.ProfileEntryFilter{Kind: store.ProfileEntryCase})
	if err != nil || len(cases) != 1 || cases[0].RoleID != nil {
		t.Errorf("cases after deleting the role = %+v, %v; want the case kept, untied", cases, err)
	}
	var changes int
	service.pool.QueryRow(context.Background(), `SELECT count(*) FROM changes WHERE entity_type = 'profile_entry'`).Scan(&changes)
	if changes != 5 {
		t.Errorf("profile entry changes = %d, want 5 (2 creates, a confirm, an update, a delete)", changes)
	}
}
