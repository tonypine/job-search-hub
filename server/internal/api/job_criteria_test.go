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
		"eligible_location_terms":["LATAM"],"ineligible_location_terms":[],"minimum_yearly_pay":{"amount":100000,"currency":"CAD"},"refuse_hourly_work":true}`

	status, answer := send(t, http.MethodPut, service.url+"/v1/job-criteria", ownerToken, body)
	if status != http.StatusOK {
		t.Fatalf("save: %d %s", status, answer)
	}
	status, answer = send(t, http.MethodGet, service.url+"/v1/job-criteria", ownerToken, "")
	var read store.SavedJobCriteria
	if err := json.Unmarshal(answer, &read); status != http.StatusOK || err != nil ||
		read.Criteria.MinimumYearlyPay == nil || read.Criteria.MinimumYearlyPay.Currency != "CAD" || read.Criteria.SearchTerms[0] != "react" {
		t.Fatalf("read: %d %s", status, answer)
	}

	for name, invalid := range map[string]string{
		"an unknown field": `{"salary_floor":100000}`,
		"negative pay":     `{"minimum_yearly_pay":{"amount":-5,"currency":"USD"}}`,
	} {
		if status, answer := send(t, http.MethodPut, service.url+"/v1/job-criteria", ownerToken, invalid); status != http.StatusBadRequest || !strings.Contains(string(answer), "error") {
			t.Errorf("%s: %d %s, want 400", name, status, answer)
		}
	}
}
