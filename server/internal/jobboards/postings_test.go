package jobboards_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

func startPostingProviders(t *testing.T) *jobboards.Verifier {
	t.Helper()
	routes := http.NewServeMux()
	// Like the real APIs, Greenhouse and Ashby publish pay only when asked.
	routes.HandleFunc("GET /v1/boards/acme/jobs", func(w http.ResponseWriter, r *http.Request) {
		pay := ""
		if r.URL.Query().Get("pay_transparency") == "true" {
			pay = `"pay_input_ranges":[{"min_cents":15200000,"max_cents":19000000,"currency_type":"USD","title":"US Pay Range","blurb":""}],`
		}
		w.Write([]byte(`{"jobs":[{"id":4721289005,"title":"Frontend Engineer","absolute_url":"https://job-boards.greenhouse.io/acme/jobs/4721289005",
			"location":{"name":"Remote (Americas)"},` + pay + `
			"first_published":"2026-08-04T14:21:42-04:00","departments":[{"id":1,"name":"Engineering"}],
			"offices":[{"id":1,"name":"Remote (Canada)"},{"id":2,"name":"Remote (Americas) "}],
			"metadata":[{"id":1,"name":"Employment Type","value":"Full-time","value_type":"single_select"}],
			"content":"&lt;h2&gt;About the role&lt;/h2&gt;&lt;p&gt;Build &amp;amp; ship.&lt;/p&gt;&lt;ul&gt;&lt;li&gt;React&lt;/li&gt;&lt;li&gt;TypeScript&lt;/li&gt;&lt;/ul&gt;"}]}`))
	})
	routes.HandleFunc("GET /v0/postings/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[{"id":"69616656","text":"Full-stack Engineer","hostedUrl":"https://jobs.lever.co/acme/69616656","workplaceType":"remote",
			"categories":{"location":"Toronto, Ontario","allLocations":["Toronto, Ontario","Vancouver, British Columbia"],
				"commitment":"Full-time","department":"Engineering","team":"Payments"},
			"createdAt":1786394912472,"salaryRange":{"min":120000,"max":150000,"currency":"CAD","interval":"per-year-salary"},
			"description":"<div><b>About the role</b></div><div>We build <b>payments</b>.</div><ul><li>Remote first</li><li>Small team</li></ul>",
			"descriptionPlain":"About the role\nWe build payments.\nRemote first\nSmall team",
			"lists":[{"text":"You will","content":"<li>Ship features</li><li>Talk to customers</li>"}],
			"additional":"<h3>Benefits</h3><ul><li>Health</li><li>Equipment</li></ul>","additionalPlain":"Benefits\nHealth\nEquipment"}]`))
	})
	routes.HandleFunc("GET /posting-api/job-board/acme", func(w http.ResponseWriter, r *http.Request) {
		compensation := ""
		if r.URL.Query().Get("includeCompensation") == "true" {
			compensation = `"compensation":{"compensationTierSummary":"$230K • Offers Equity","compensationTiers":[{"title":null,"components":[
				{"compensationType":"EquityPercentage","interval":"NONE","currencyCode":null,"minValue":null,"maxValue":null},
				{"compensationType":"Salary","interval":"1 YEAR","currencyCode":"USD","minValue":230000,"maxValue":230000}]}]},`
		}
		w.Write([]byte(`{"jobs":[
			{"id":"c3fe","title":"Senior Product Engineer","location":"Americas","workplaceType":"Remote","jobUrl":"https://jobs.ashbyhq.com/acme/c3fe",` + compensation + `
				"employmentType":"FullTime","department":"Engineering","publishedAt":"2026-09-02T10:22:06.450+00:00",
				"secondaryLocations":[{"location":"EMEA","address":{}}],"isListed":true,
				"descriptionHtml":"<h2>About the role</h2><p>Build <strong>scheduling</strong>.</p><ul><li><p>Go</p></li><li><p>Swift</p></li></ul>",
				"descriptionPlain":"ABOUT THE ROLE\n\nBuild scheduling.\n\n- Go\n\n- Swift"},
			{"id":"b7aa","title":"Support Engineer","location":"Remote","jobUrl":"https://jobs.ashbyhq.com/acme/b7aa","descriptionPlain":"No pay here.","isListed":true},
			{"id":"hidden","title":"Internal role","location":"Remote","jobUrl":"https://jobs.ashbyhq.com/acme/hidden","descriptionPlain":"","isListed":false}]}`))
	})
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return &jobboards.Verifier{HTTPClient: &http.Client{Timeout: time.Second}, GreenhouseAPIBase: server.URL, LeverAPIBase: server.URL, AshbyAPIBase: server.URL, AshbyBoardBase: server.URL}
}

func TestGreenhousePostingsKeepTheirHeadingsAndListsAsMarkdown(t *testing.T) {
	postings, err := startPostingProviders(t).FetchPostings(context.Background(), jobboards.Greenhouse, "acme")
	if err != nil || len(postings) != 1 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	posting := postings[0]
	if posting.ExternalID != "4721289005" || posting.Location != "Remote (Americas)" || posting.URL != "https://job-boards.greenhouse.io/acme/jobs/4721289005" {
		t.Fatalf("posting = %+v", posting)
	}
	if posting.Description != "### About the role\n\nBuild & ship.\n\n- React\n- TypeScript" {
		t.Fatalf("description = %q", posting.Description)
	}
	if len(posting.Raw) == 0 {
		t.Fatal("the raw posting was not kept")
	}
}

func TestLeverPostingsKeepTheirHeadingsAndListsAsMarkdown(t *testing.T) {
	postings, err := startPostingProviders(t).FetchPostings(context.Background(), jobboards.Lever, "acme")
	if err != nil || len(postings) != 1 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	posting := postings[0]
	if posting.Title != "Full-stack Engineer" || posting.WorkplaceType != "remote" || posting.Location != "Toronto, Ontario" {
		t.Fatalf("posting = %+v", posting)
	}
	want := "**About the role**\n\nWe build **payments**.\n\n- Remote first\n- Small team\n\n### You will\n\n- Ship features\n- Talk to customers\n\n### Benefits\n\n- Health\n- Equipment"
	if posting.Description != want {
		t.Fatalf("description = %q, want %q", posting.Description, want)
	}
}

func TestAshbyPostingsKeepTheirHeadingsAndListsAsMarkdown(t *testing.T) {
	postings, err := startPostingProviders(t).FetchPostings(context.Background(), jobboards.Ashby, "acme")
	if err != nil || len(postings) != 2 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	if want := "### About the role\n\nBuild **scheduling**.\n\n- Go\n- Swift"; postings[0].Description != want {
		t.Errorf("description = %q, want %q", postings[0].Description, want)
	}
	if postings[1].Description != "No pay here." {
		t.Errorf("a posting without HTML stored %q, want its plain text", postings[1].Description)
	}
}

func TestAshbyDropsUnlistedPostings(t *testing.T) {
	postings, err := startPostingProviders(t).FetchPostings(context.Background(), jobboards.Ashby, "acme")
	if err != nil || len(postings) != 2 || postings[0].ExternalID != "c3fe" || postings[0].Location != "Americas" || postings[1].ExternalID != "b7aa" {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
}

func TestEachProviderPublishesPayAndHiringFacts(t *testing.T) {
	verifier := startPostingProviders(t)
	publishedAt := func(text string) time.Time {
		parsed, err := time.Parse(time.RFC3339, text)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	for _, want := range []struct {
		provider       string
		pay            store.Pay
		employmentType string
		department     string
		otherLocations []string
		publishedAt    time.Time
	}{
		{jobboards.Greenhouse, store.Pay{Ranges: []store.PayRange{{Label: "US Pay Range", Min: 152000, Max: 190000, Currency: "USD"}}},
			"Full-time", "Engineering", []string{"Remote (Canada)"}, publishedAt("2026-08-04T14:21:42-04:00")},
		{jobboards.Lever, store.Pay{Ranges: []store.PayRange{{Min: 120000, Max: 150000, Currency: "CAD", Interval: "year"}}},
			"Full-time", "Engineering", []string{"Vancouver, British Columbia"}, time.UnixMilli(1786394912472)},
		{jobboards.Ashby, store.Pay{Ranges: []store.PayRange{{Min: 230000, Max: 230000, Currency: "USD", Interval: "year"}}, Summary: "$230K • Offers Equity"},
			"Full-time", "Engineering", []string{"EMEA"}, publishedAt("2026-09-02T10:22:06.450Z")},
	} {
		postings, err := verifier.FetchPostings(context.Background(), want.provider, "acme")
		if err != nil {
			t.Fatalf("%s: %v", want.provider, err)
		}
		facts := postings[0].BoardFacts
		if facts.Pay == nil || !reflect.DeepEqual(*facts.Pay, want.pay) {
			t.Errorf("%s pay = %+v, want %+v", want.provider, facts.Pay, want.pay)
		}
		if facts.EmploymentType != want.employmentType || facts.Department != want.department || !reflect.DeepEqual(facts.OtherLocations, want.otherLocations) {
			t.Errorf("%s facts = %+v", want.provider, facts)
		}
		if facts.PublishedAt == nil || !facts.PublishedAt.Equal(want.publishedAt) {
			t.Errorf("%s published at %v, want %v", want.provider, facts.PublishedAt, want.publishedAt)
		}
	}

	ashby, _ := verifier.FetchPostings(context.Background(), jobboards.Ashby, "acme")
	if ashby[1].Pay != nil {
		t.Errorf("a posting without published pay got %+v", ashby[1].Pay)
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

func TestParsePostingURL(t *testing.T) {
	for raw, want := range map[string]jobboards.PostingReference{
		"https://job-boards.greenhouse.io/tailscale/jobs/4721289005": {Provider: "greenhouse", BoardToken: "tailscale", PostingID: "4721289005"},
		"https://boards.greenhouse.io/stripe/jobs/123?gh_src=x":      {Provider: "greenhouse", BoardToken: "stripe", PostingID: "123"},
		"https://jobs.lever.co/waveapps/6961-abc":                    {Provider: "lever", BoardToken: "waveapps", PostingID: "6961-abc"},
		"https://jobs.lever.co/waveapps/6961-abc/apply":              {Provider: "lever", BoardToken: "waveapps", PostingID: "6961-abc"},
		"https://jobs.ashbyhq.com/revenuecat/c3fe34a4":               {Provider: "ashby", BoardToken: "revenuecat", PostingID: "c3fe34a4"},
	} {
		got, ok := jobboards.ParsePostingURL(raw)
		if !ok || got != want {
			t.Errorf("%s: got %+v, %v; want %+v", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"https://acme.com/careers/42", "https://jobs.lever.co/waveapps", "https://job-boards.greenhouse.io/tailscale"} {
		if got, ok := jobboards.ParsePostingURL(raw); ok {
			t.Errorf("%s was recognized as %+v", raw, got)
		}
	}
}

func TestParseBoardURL(t *testing.T) {
	for raw, want := range map[string][2]string{
		"https://job-boards.greenhouse.io/acmecomputing":                       {"greenhouse", "acmecomputing"},
		"https://boards.greenhouse.io/stripe/jobs/123?gh_src=x":                {"greenhouse", "stripe"},
		"https://boards.greenhouse.io/embed/job_board?for=initech":             {"greenhouse", "initech"},
		"https://jobs.lever.co/waveapps/":                                      {"lever", "waveapps"},
		"https://jobs.ashbyhq.com/acme.io":                                     {"ashby", "acme.io"},
		"https://jobs.ashbyhq.com/globex/089c4729-a8b3-4a69-98f3-ceecdf17d369": {"ashby", "globex"},
	} {
		provider, boardToken, ok := jobboards.ParseBoardURL(raw)
		if !ok || [2]string{provider, boardToken} != want {
			t.Errorf("%s: got %s/%s, %v; want %v", raw, provider, boardToken, ok, want)
		}
	}
	for _, raw := range []string{"https://acme.com/careers", "https://jobs.lever.co/", "https://boards.greenhouse.io/embed/job_board", "::"} {
		if provider, boardToken, ok := jobboards.ParseBoardURL(raw); ok {
			t.Errorf("%s was recognized as %s/%s", raw, provider, boardToken)
		}
	}
}

func TestFetchPostingReadsOnePostingPerProvider(t *testing.T) {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /v1/boards/acme/jobs/7", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"id":7,"title":"Frontend Engineer","absolute_url":"https://job-boards.greenhouse.io/acme/jobs/7","location":{"name":"Remote"},"content":"&lt;p&gt;Hi&lt;/p&gt;"}`))
	})
	routes.HandleFunc("GET /v0/postings/acme/abc", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"id":"abc","text":"Full-stack Engineer","hostedUrl":"https://jobs.lever.co/acme/abc","categories":{"location":"Toronto"},"descriptionPlain":"About."}`))
	})
	routes.HandleFunc("GET /posting-api/job-board/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"jobs":[{"id":"x1","title":"Product Engineer","location":"Americas","jobUrl":"https://jobs.ashbyhq.com/acme/x1","descriptionPlain":"","isListed":true}]}`))
	})
	server := httptest.NewServer(routes)
	defer server.Close()
	verifier := &jobboards.Verifier{HTTPClient: &http.Client{Timeout: time.Second}, GreenhouseAPIBase: server.URL, LeverAPIBase: server.URL, AshbyAPIBase: server.URL}

	for reference, wantTitle := range map[jobboards.PostingReference]string{
		{Provider: "greenhouse", BoardToken: "acme", PostingID: "7"}: "Frontend Engineer",
		{Provider: "lever", BoardToken: "acme", PostingID: "abc"}:    "Full-stack Engineer",
		{Provider: "ashby", BoardToken: "acme", PostingID: "x1"}:     "Product Engineer",
	} {
		posting, err := verifier.FetchPosting(context.Background(), reference)
		if err != nil || posting.Title != wantTitle || posting.URL == "" {
			t.Errorf("%+v: %+v, %v", reference, posting, err)
		}
	}
	lever, _ := verifier.FetchPosting(context.Background(), jobboards.PostingReference{Provider: "lever", BoardToken: "acme", PostingID: "abc"})
	if lever.Description != "About." {
		t.Errorf("a Lever posting without HTML stored %q, want its plain text", lever.Description)
	}
	if _, err := verifier.FetchPosting(context.Background(), jobboards.PostingReference{Provider: "ashby", BoardToken: "acme", PostingID: "gone"}); err == nil {
		t.Error("expected an error for a posting the Ashby board doesn't list")
	}
}
