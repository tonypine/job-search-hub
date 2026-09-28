package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheCriteriaAreSavedAndReadOverREST(t *testing.T) {
	service := startAPI(t)
	body := `{"roles":["Product Engineer"],"search_terms":["react"],"technologies":["TypeScript"],"seniority_levels":["Senior"],
		"eligible_location_terms":["LATAM"],"ineligible_location_terms":[],"take_home":{"currency":"BRL","minimum_monthly":16000,"target_monthly":44000,
		"clt":{"share":0.73,"payments_per_year":13.33},"pj":{"share":0.82,"payments_per_year":12},"foreign_contractor":{"share":0.84,"payments_per_year":12}},"refuse_hourly_work":true}`

	status, answer := send(t, http.MethodPut, service.url+"/v1/job-criteria", ownerToken, body)
	if status != http.StatusOK {
		t.Fatalf("save: %d %s", status, answer)
	}
	status, answer = send(t, http.MethodGet, service.url+"/v1/job-criteria", ownerToken, "")
	var read store.SavedJobCriteria
	if err := json.Unmarshal(answer, &read); status != http.StatusOK || err != nil ||
		read.Criteria.TakeHome == nil || read.Criteria.TakeHome.CLT.PaymentsPerYear != 13.33 || read.Criteria.SearchTerms[0] != "react" {
		t.Fatalf("read: %d %s", status, answer)
	}

	for name, invalid := range map[string]string{
		"an unknown field": `{"salary_floor":100000}`,
		"a share above one": `{"take_home":{"currency":"BRL","minimum_monthly":1,"target_monthly":1,"clt":{"share":2,"payments_per_year":12},
			"pj":{"share":0.8,"payments_per_year":12},"foreign_contractor":{"share":0.8,"payments_per_year":12}}}`,
	} {
		if status, answer := send(t, http.MethodPut, service.url+"/v1/job-criteria", ownerToken, invalid); status != http.StatusBadRequest || !strings.Contains(string(answer), "error") {
			t.Errorf("%s: %d %s, want 400", name, status, answer)
		}
	}
}
