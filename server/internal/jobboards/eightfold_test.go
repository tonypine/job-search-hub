package jobboards_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
)

func TestAnEightfoldBoardIsSearchedByTheOwnersTermsAndEachDescriptionReadOnce(t *testing.T) {
	var detailReads atomic.Int32
	routes := http.NewServeMux()
	routes.HandleFunc("GET /api/pcsx/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("domain") != "acme.com" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.URL.Query().Get("query") {
		case "":
			w.Write([]byte(`{"data":{"count":880,"positions":[]}}`))
		case "react":
			w.Write([]byte(`{"data":{"count":2,"positions":[
				{"id":11,"name":"Senior Frontend Engineer","locations":["São Paulo,Brazil","Remote"],"postedTs":1790000000,"department":"IT",
				 "workLocationOption":"remote_global","positionUrl":"/careers/job/11"},
				{"id":12,"name":"Frontend Developer","locations":["Buenos Aires,Argentina"],"workLocationOption":"hybrid","positionUrl":"/careers/job/12"}]}}`))
		case "frontend":
			w.Write([]byte(`{"data":{"count":1,"positions":[{"id":11,"name":"Senior Frontend Engineer","locations":["São Paulo,Brazil"],"positionUrl":"/careers/job/11"}]}}`))
		}
	})
	routes.HandleFunc("GET /api/pcsx/position_details", func(w http.ResponseWriter, r *http.Request) {
		detailReads.Add(1)
		w.Write([]byte(`{"data":{"jobDescription":"<p>Build the checkout &amp; its UI.</p>"}}`))
	})
	server := httptest.NewServer(routes)
	defer server.Close()
	verifier := &jobboards.Verifier{
		HTTPClient: &http.Client{Timeout: time.Second}, EightfoldAPIBase: server.URL,
		SearchTerms: func(context.Context) []string { return []string{"react", "frontend"} },
	}
	ctx := context.Background()

	verification, err := verifier.Verify(ctx, jobboards.Eightfold, "acme/acme.com")
	if err != nil || !verification.Verified || *verification.OpenPostingCount != 880 {
		t.Fatalf("verify = %+v, %v", verification, err)
	}
	if _, err := verifier.Verify(ctx, jobboards.Eightfold, "acme"); err == nil {
		t.Error("a token without the company domain was accepted")
	}

	postings, err := verifier.FetchPostings(ctx, jobboards.Eightfold, "acme/acme.com")
	if err != nil || len(postings) != 2 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	first := postings[0]
	if first.ExternalID != "11" || first.Location != "São Paulo,Brazil" || first.WorkplaceType != "Remote" || first.Department != "IT" ||
		first.URL != server.URL+"/careers/job/11?domain=acme.com" || first.Description != "Build the checkout & its UI." || first.PublishedAt == nil {
		t.Errorf("first = %+v", first)
	}
	if postings[1].WorkplaceType != "Hybrid" {
		t.Errorf("second = %+v", postings[1])
	}
	if _, err := verifier.FetchPostings(ctx, jobboards.Eightfold, "acme/acme.com"); err != nil || detailReads.Load() != 2 {
		t.Errorf("a second poll read %d descriptions in all, %v; want each read once", detailReads.Load(), err)
	}
}
