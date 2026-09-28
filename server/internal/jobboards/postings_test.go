package jobboards_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
)

func startPostingProviders(t *testing.T) *jobboards.Verifier {
	t.Helper()
	routes := http.NewServeMux()
	routes.HandleFunc("GET /v1/boards/acme/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"jobs":[{"id":4721289005,"title":"Frontend Engineer","absolute_url":"https://job-boards.greenhouse.io/acme/jobs/4721289005",
			"location":{"name":"Remote (Americas)"},
			"content":"&lt;p&gt;Build &amp;amp; ship.&lt;/p&gt;&lt;ul&gt;&lt;li&gt;React&lt;/li&gt;&lt;li&gt;TypeScript&lt;/li&gt;&lt;/ul&gt;"}]}`))
	})
	routes.HandleFunc("GET /v0/postings/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[{"id":"69616656","text":"Full-stack Engineer","hostedUrl":"https://jobs.lever.co/acme/69616656","workplaceType":"remote",
			"categories":{"location":"Toronto, Ontario"},"descriptionPlain":"About the role.",
			"lists":[{"text":"You will","content":"<li>Ship features</li><li>Talk to customers</li>"}],"additionalPlain":"Benefits."}]`))
	})
	routes.HandleFunc("GET /posting-api/job-board/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"jobs":[
			{"id":"c3fe","title":"Senior Product Engineer","location":"Americas","workplaceType":"Remote","jobUrl":"https://jobs.ashbyhq.com/acme/c3fe","descriptionPlain":"Plain text.","isListed":true},
			{"id":"hidden","title":"Internal role","location":"Remote","jobUrl":"https://jobs.ashbyhq.com/acme/hidden","descriptionPlain":"","isListed":false}]}`))
	})
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return &jobboards.Verifier{HTTPClient: &http.Client{Timeout: time.Second}, GreenhouseAPIBase: server.URL, LeverAPIBase: server.URL, AshbyAPIBase: server.URL, AshbyBoardBase: server.URL}
}

func TestGreenhousePostingsBecomePlainText(t *testing.T) {
	postings, err := startPostingProviders(t).FetchPostings(context.Background(), jobboards.Greenhouse, "acme")
	if err != nil || len(postings) != 1 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	posting := postings[0]
	if posting.ExternalID != "4721289005" || posting.Location != "Remote (Americas)" || posting.URL != "https://job-boards.greenhouse.io/acme/jobs/4721289005" {
		t.Fatalf("posting = %+v", posting)
	}
	if posting.Description != "Build &amp; ship.\nReact\nTypeScript" && posting.Description != "Build & ship.\nReact\nTypeScript" {
		t.Fatalf("description = %q", posting.Description)
	}
	if len(posting.Raw) == 0 {
		t.Fatal("the raw posting was not kept")
	}
}

func TestLeverPostingsIncludeTheirLists(t *testing.T) {
	postings, err := startPostingProviders(t).FetchPostings(context.Background(), jobboards.Lever, "acme")
	if err != nil || len(postings) != 1 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	posting := postings[0]
	if posting.Title != "Full-stack Engineer" || posting.WorkplaceType != "remote" || posting.Location != "Toronto, Ontario" {
		t.Fatalf("posting = %+v", posting)
	}
	for _, want := range []string{"About the role.", "You will\nShip features\nTalk to customers", "Benefits."} {
		if !strings.Contains(posting.Description, want) {
			t.Errorf("description lacks %q:\n%s", want, posting.Description)
		}
	}
}

func TestAshbyDropsUnlistedPostings(t *testing.T) {
	postings, err := startPostingProviders(t).FetchPostings(context.Background(), jobboards.Ashby, "acme")
	if err != nil || len(postings) != 1 || postings[0].ExternalID != "c3fe" || postings[0].Location != "Americas" {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
}

func TestABoardWithoutAPostingAPIIsReportedAsSuch(t *testing.T) {
	_, err := startPostingProviders(t).FetchPostings(context.Background(), jobboards.Ashby, "pageonly")
	if !errors.Is(err, jobboards.ErrPostingAPIOff) {
		t.Fatalf("err = %v, want ErrPostingAPIOff", err)
	}
}

func TestLivePostings(t *testing.T) {
	if os.Getenv("HUB_LIVE_TEST") != "1" {
		t.Skip("set HUB_LIVE_TEST=1 to call the real provider APIs")
	}
	verifier := jobboards.NewVerifier()
	for provider, boardToken := range map[string]string{jobboards.Greenhouse: "tailscale", jobboards.Lever: "waveapps", jobboards.Ashby: "revenuecat"} {
		postings, err := verifier.FetchPostings(context.Background(), provider, boardToken)
		if err != nil || len(postings) == 0 || postings[0].Title == "" || postings[0].URL == "" || postings[0].Description == "" {
			t.Errorf("%s/%s: %d postings, err %v", provider, boardToken, len(postings), err)
		}
	}
}
