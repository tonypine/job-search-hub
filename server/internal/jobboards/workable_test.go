package jobboards_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
)

func TestAWorkableBoardIsVerifiedAndItsJobsRead(t *testing.T) {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /api/v1/widget/accounts/acme", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"Acme","jobs":[
			{"title":"Senior Frontend Engineer","shortcode":"AB12","url":"https://apply.workable.com/j/AB12","employment_type":"Full-time",
			 "telecommuting":"True","department":"Engineering","published_on":"2026-09-20",
			 "locations":[{"city":"São Paulo","region":"SP","country":"Brazil"},{"city":"","region":"","country":"Portugal"}],
			 "description":"<p>Build the app&#39;s UI.</p>"},
			{"title":"Office Manager","shortcode":"CD34","url":"https://apply.workable.com/j/CD34","telecommuting":false,
			 "locations":[{"city":"Lisbon","country":"Portugal"}],"description":""}]}`))
	})
	server := httptest.NewServer(routes)
	defer server.Close()
	verifier := &jobboards.Verifier{HTTPClient: &http.Client{Timeout: time.Second}, WorkableAPIBase: server.URL}
	ctx := context.Background()

	verification, err := verifier.Verify(ctx, jobboards.Workable, "acme")
	if err != nil || !verification.Verified || verification.OpenPostingCount == nil || *verification.OpenPostingCount != 2 ||
		verification.BoardURL != "https://apply.workable.com/acme/" {
		t.Fatalf("verify = %+v, %v", verification, err)
	}
	if unknown, err := verifier.Verify(ctx, jobboards.Workable, "nobody"); err != nil || unknown.Verified {
		t.Fatalf("an unknown account = %+v, %v", unknown, err)
	}

	postings, err := verifier.FetchPostings(ctx, jobboards.Workable, "acme")
	if err != nil || len(postings) != 2 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	first := postings[0]
	if first.ExternalID != "AB12" || first.Title != "Senior Frontend Engineer" || first.Location != "São Paulo, SP, Brazil" ||
		len(first.OtherLocations) != 1 || first.OtherLocations[0] != "Portugal" || first.WorkplaceType != "Remote" ||
		first.EmploymentType != "Full-time" || first.Description != "Build the app's UI." || first.PublishedAt == nil {
		t.Errorf("first = %+v", first)
	}
	if postings[1].WorkplaceType != "" || postings[1].Location != "Lisbon, Portugal" {
		t.Errorf("second = %+v", postings[1])
	}
}
