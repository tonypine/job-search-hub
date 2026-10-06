package jobboards_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
)

var searchedAt = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func himalayasJob(guid string, published time.Time, restrictions, salary string) string {
	return fmt.Sprintf(`{"guid":"https://himalayas.app/companies/acme/jobs/%[1]s","title":"Senior Frontend Engineer %[1]s","companyName":"Acme",
		"applicationLink":"https://himalayas.app/companies/acme/jobs/%[1]s","employmentType":"Full Time",%[4]s
		"locationRestrictions":%[3]s,"description":"<h2>About</h2><p>Build <strong>React</strong> apps.</p><ul><li>TypeScript</li></ul>",
		"pubDate":%[2]d,"expiryDate":%[5]d}`, guid, published.Unix(), restrictions, salary, published.Add(60*24*time.Hour).Unix())
}

func startHimalayas(t *testing.T, pages map[string]string) (*jobboards.Verifier, *[]url.Values) {
	t.Helper()
	var mutex sync.Mutex
	var queries []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/api/search" {
			http.NotFound(w, r)
			return
		}
		mutex.Lock()
		queries = append(queries, r.URL.Query())
		mutex.Unlock()
		w.Write([]byte(`{"jobs":[` + pages[r.URL.Query().Get("page")] + `]}`))
	}))
	t.Cleanup(server.Close)
	return &jobboards.Verifier{HTTPClient: &http.Client{Timeout: time.Second}, HimalayasAPIBase: server.URL}, &queries
}

func TestHimalayasSearchStopsAtThePageWithNothingRecent(t *testing.T) {
	recent, old := searchedAt.Add(-24*time.Hour), searchedAt.Add(-30*24*time.Hour)
	verifier, queries := startHimalayas(t, map[string]string{
		"1": himalayasJob("a", recent, `[]`, `"minSalary":8000,"maxSalary":10000,"currency":"USD","salaryPeriod":"monthly",`) + "," +
			himalayasJob("b", old, `["Brazil","Argentina"]`, ``),
		"2": himalayasJob("c", recent, `["Brazil","Argentina","Chile","Mexico","Colombia"]`, `"minSalary":120000,"maxSalary":null,"currency":"CAD","salaryPeriod":"annual",`),
		"3": himalayasJob("d", old, `[]`, ``),
		"4": himalayasJob("e", recent, `[]`, ``),
	})

	postings, err := verifier.SearchHimalayas(context.Background(), jobboards.HimalayasSearch{
		Term: "react", Country: "Brazil", PublishedAfter: searchedAt.Add(-14 * 24 * time.Hour), MaximumPages: 5,
	})
	if err != nil || len(postings) != 2 {
		t.Fatalf("postings = %+v, %v; want a and c", postings, err)
	}
	if len(*queries) != 3 {
		t.Fatalf("read %d pages, want 3: page 3 had nothing recent", len(*queries))
	}
	first := (*queries)[0]
	if first.Get("q") != "react" || first.Get("country") != "Brazil" || first.Get("sort") != "recent" || first.Get("page") != "1" {
		t.Fatalf("query = %v", first)
	}

	worldwide, restricted := postings[0], postings[1]
	if worldwide.Location != "Worldwide" || worldwide.CompanyName != "Acme" || worldwide.EmploymentType != "Full-time" ||
		worldwide.WorkplaceType != "Remote" || worldwide.Description != "### About\n\nBuild **React** apps.\n\n- TypeScript" {
		t.Fatalf("worldwide posting = %+v", worldwide)
	}
	if worldwide.Pay == nil || worldwide.Pay.Ranges[0].Interval != "month" || worldwide.Pay.Ranges[0].Max != 10000 {
		t.Fatalf("monthly pay = %+v", worldwide.Pay)
	}
	if worldwide.PublishedAt == nil || worldwide.ExpiresAt == nil || !worldwide.ExpiresAt.After(*worldwide.PublishedAt) {
		t.Fatalf("dates = %v, %v", worldwide.PublishedAt, worldwide.ExpiresAt)
	}
	if restricted.Location != "5 countries" || !reflect.DeepEqual(restricted.OtherLocations, []string{"Brazil", "Argentina", "Chile", "Mexico", "Colombia"}) {
		t.Fatalf("restricted posting location = %q, %v", restricted.Location, restricted.OtherLocations)
	}
	if pay := restricted.Pay; pay == nil || pay.Ranges[0].Min != 120000 || pay.Ranges[0].Max != 120000 || pay.Ranges[0].Interval != "year" {
		t.Fatalf("one-bound pay = %+v", pay)
	}
}

func TestAFewCountriesAreNamedAsTheLocation(t *testing.T) {
	verifier, _ := startHimalayas(t, map[string]string{"1": himalayasJob("a", searchedAt, `["Brazil","Portugal"]`, ``)})

	postings, err := verifier.SearchHimalayas(context.Background(), jobboards.HimalayasSearch{Term: "react", Country: "Brazil", MaximumPages: 1})
	if err != nil || len(postings) != 1 || postings[0].Location != "Brazil, Portugal" || postings[0].OtherLocations != nil || postings[0].Pay != nil {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
}
