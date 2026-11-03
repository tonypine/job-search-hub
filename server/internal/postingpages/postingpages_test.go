package postingpages_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/postingpages"
)

// The pages under testdata are made up; no real company's careers page is
// copied here.
func parseFixture(t *testing.T, name, pageURL string) postingpages.Page {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(pageURL)
	if err != nil {
		t.Fatal(err)
	}
	return postingpages.ParsePage(parsed, body)
}

func TestAPageLinkingItsPostingOnLeverNamesThatPosting(t *testing.T) {
	// The page also links another of the board's postings; its own id is in
	// the page's URL.
	page := parseFixture(t, "careers-page-lever.html", "https://initech.example/careers/jobs/0f6c2a1e-1111-4222-8333-944455556666")
	want := jobboards.PostingReference{Provider: jobboards.Lever, BoardToken: "initech", PostingID: "0f6c2a1e-1111-4222-8333-944455556666"}
	if page.Reference == nil || *page.Reference != want {
		t.Fatalf("reference = %+v, want %+v", page.Reference, want)
	}
	// Should Lever not answer, the page's own text is there too.
	if page.Posting.Title != "Senior Platform Engineer" || strings.Contains(page.Posting.Description, "Designer") {
		t.Fatalf("posting = %+v", page.Posting)
	}
}

func TestAnEmbeddedGreenhouseBoardNamesThePostingOfTheGhJid(t *testing.T) {
	page := parseFixture(t, "careers-page-greenhouse-embed.html", "https://initech.example/careers?gh_jid=4012345")
	want := jobboards.PostingReference{Provider: jobboards.Greenhouse, BoardToken: "initech", PostingID: "4012345"}
	if page.Reference == nil || *page.Reference != want {
		t.Fatalf("reference = %+v, want %+v", page.Reference, want)
	}
	if page := parseFixture(t, "careers-page-greenhouse-embed.html", "https://initech.example/careers"); page.Reference != nil {
		t.Fatalf("a board with no gh_jid named %+v", page.Reference)
	}
}

func TestAListOfPostingsNamesNoneOfThem(t *testing.T) {
	page := parseFixture(t, "careers-page-job-list.html", "https://initech.example/careers")
	if page.Reference != nil {
		t.Fatalf("reference = %+v, want none", page.Reference)
	}
	if page := parseFixture(t, "careers-page-job-list.html", "https://initech.example/careers/222"); page.Reference == nil || page.Reference.PostingID != "222" {
		t.Fatalf("the page of posting 222 named %+v", page.Reference)
	}
}

func TestJobPostingDataGivesTheTitleLocationPayAndText(t *testing.T) {
	page := parseFixture(t, "careers-page-job-posting.html", "https://initech.example/jobs/be-77")
	posting := page.Posting
	if page.Reference != nil || page.ReadFrom != postingpages.ReadFromStructuredData {
		t.Fatalf("page = %+v", page)
	}
	if posting.Title != "Backend Engineer" || posting.Location != "Lisbon, PT" || len(posting.OtherLocations) != 1 || posting.OtherLocations[0] != "Porto, PT" ||
		posting.EmploymentType != "Full-time" || posting.ExternalID != "BE-77" || posting.URL != "https://initech.example/jobs/be-77" {
		t.Fatalf("posting = %+v", posting)
	}
	if posting.PublishedAt == nil || posting.PublishedAt.Format("2006-01-02") != "2026-10-30" {
		t.Fatalf("published = %v", posting.PublishedAt)
	}
	if posting.Pay == nil || len(posting.Pay.Ranges) != 1 {
		t.Fatalf("pay = %+v", posting.Pay)
	}
	if got := posting.Pay.Ranges[0]; got.Min != 60000 || got.Max != 80000 || got.Currency != "EUR" || got.Interval != "year" {
		t.Fatalf("pay range = %+v", got)
	}
	if want := "Build the **billing** service.\n\n- Go\n- Postgres"; posting.Description != want {
		t.Fatalf("description = %q, want %q", posting.Description, want)
	}
}

func TestAPlainPageGivesItsMainContentAlone(t *testing.T) {
	page := parseFixture(t, "careers-page-plain.html", "https://initech.example/careers/sre")
	if page.ReadFrom != postingpages.ReadFromPageText || page.Posting.Title != "Site Reliability Engineer" {
		t.Fatalf("page = %+v", page)
	}
	text := page.Posting.Description
	for _, kept := range []string{"### Site Reliability Engineer", "### About the role", "make deploys boring", "- Kubernetes and Terraform"} {
		if !strings.Contains(text, kept) {
			t.Errorf("the text lacks %q:\n%s", kept, text)
		}
	}
	for _, dropped := range []string{"cookies", "Accept all", "About us", "Copyright", "Privacy", "Apply", "analytics", "color:red"} {
		if strings.Contains(text, dropped) {
			t.Errorf("the text keeps %q:\n%s", dropped, text)
		}
	}
}

func TestAPageDrawnWithJavaScriptHasNoText(t *testing.T) {
	page := parseFixture(t, "careers-page-javascript.html", "https://initech.example/careers/42")
	if page.Reference != nil || page.Posting.Description != "" || page.Posting.Title != "Careers" {
		t.Fatalf("page = %+v", page)
	}
}

func TestReadPageFetchesThePage(t *testing.T) {
	plain, err := os.ReadFile("testdata/careers-page-plain.html")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/careers/sre":
			if !strings.Contains(r.Header.Get("User-Agent"), "JobSearchHub") {
				t.Errorf("user agent = %q", r.Header.Get("User-Agent"))
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(plain)
		case "/careers/posting.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.7"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	reader := postingpages.NewReader()

	page, err := reader.ReadPage(context.Background(), server.URL+"/careers/sre")
	if err != nil || page.Posting.Title != "Site Reliability Engineer" || page.Posting.URL != server.URL+"/careers/sre" {
		t.Fatalf("page = %+v, %v", page, err)
	}
	if _, err := reader.ReadPage(context.Background(), server.URL+"/careers/gone"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("a missing page: %v", err)
	}
	if _, err := reader.ReadPage(context.Background(), server.URL+"/careers/posting.pdf"); err == nil || !strings.Contains(err.Error(), "application/pdf") {
		t.Fatalf("a PDF: %v", err)
	}
	if _, err := reader.ReadPage(context.Background(), "ftp://initech.example/x"); err == nil {
		t.Fatal("an ftp link was read")
	}
}
