package postingtexts_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/postingtexts"
)

func TestJSearchAsksForTheFirstPageInTheCountryWithItsKey(t *testing.T) {
	var asked url.Values
	var key string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/search" {
			http.NotFound(writer, request)
			return
		}
		asked, key = request.URL.Query(), request.Header.Get("X-API-Key")
		writer.Write([]byte(`{"status":"OK","request_id":"r1","data":[{"job_id":"j1","job_title":"Frontend Engineer","employer_name":"Acme",
			"job_description":"Build the web app.","job_apply_link":"https://acme.example/jobs/1","job_country":"BR"}]}`))
	}))
	defer server.Close()

	postings, err := postingtexts.NewJSearch(server.URL+"/", "test-key").Search(context.Background(), "Frontend Engineer Acme", "br")

	if err != nil || len(postings) != 1 {
		t.Fatalf("postings %+v, err %v", postings, err)
	}
	if posting := postings[0]; posting.Title != "Frontend Engineer" || posting.EmployerName != "Acme" ||
		posting.Description != "Build the web app." || posting.ApplyLink != "https://acme.example/jobs/1" {
		t.Errorf("posting = %+v", posting)
	}
	if asked.Get("query") != "Frontend Engineer Acme" || asked.Get("country") != "br" || asked.Get("num_pages") != "1" || key != "test-key" {
		t.Errorf("asked %v with key %q", asked, key)
	}
}

func TestJSearchSaysWhenItRefusesTheKeyOrTheQuota(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(status)
		}))
		_, err := postingtexts.NewJSearch(server.URL, "test-key").Search(context.Background(), "Engineer", "")
		server.Close()
		if !errors.Is(err, postingtexts.ErrSearchRefused) {
			t.Errorf("%d: err = %v, want a refusal", status, err)
		}
	}
}

func TestJSearchSaysWhenItCannotBeReached(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	_, err := postingtexts.NewJSearch(server.URL, "test-key").Search(context.Background(), "Engineer", "")
	if !errors.Is(err, postingtexts.ErrSearchUnreachable) {
		t.Errorf("404: err = %v, want an unreachable search", err)
	}
	server.Close()

	_, err = postingtexts.NewJSearch(server.URL, "test-key").Search(context.Background(), "Engineer", "")
	if !errors.Is(err, postingtexts.ErrSearchUnreachable) {
		t.Errorf("closed server: err = %v, want an unreachable search", err)
	}
}

func TestJSearchTimingOutAfterTheRequestWasSentIsAnOrdinaryFailure(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)
	search := postingtexts.NewJSearch(server.URL, "test-key")
	search.HTTPClient.Timeout = 50 * time.Millisecond

	_, err := search.Search(context.Background(), "Engineer", "")

	if err == nil || errors.Is(err, postingtexts.ErrSearchUnreachable) {
		t.Errorf("err = %v, want a failure that may have reached JSearch", err)
	}
}
