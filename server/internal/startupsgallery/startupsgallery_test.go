package startupsgallery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// listPage is the remote list as the site serves it: each card links its
// company twice, once with its name.
const listPage = `<html><body><div>
<a class="card" href="../../companies/acme"><img src="x.jpg"><h3 class="framer-text">Acme</h3><p>A made-up tagline.</p></a>
<a href="../../companies/acme">Acme</a>
<a class="card" href="../../companies/globex"><h3>Globex &amp; Co</h3><p>Another tagline.</p></a>
<a class="card" href="../../companies/initech"><h3>Initech</h3></a>
<a class="card" href="../../companies/hooli"><h3>Hooli</h3></a>
<a href="../../categories/work-type/remote">Remote</a>
</div></body></html>`

func makeCompanyPage(website, careers string, extraLinks ...string) string {
	page := fmt.Sprintf(`<html><body>
<a class="button" data-framer-name="Button / Primary / 14px" href="%s"><div><p>Visit Website</p></div></a>
<a class="button" data-framer-name="Button / White / 14px" href="%s"><div><p>View   Jobs</p></div></a>
<a href="https://news.example.com/raised">Raised $10M</a>`, website, careers)
	for _, link := range extraLinks {
		page += fmt.Sprintf(`<a href="%s"><p>A posting</p></a>`, link)
	}
	return page + `</body></html>`
}

// fakeSite serves startups.gallery: its robots.txt, the remote list and a
// page per company; hooli's page fails until it's fixed.
type fakeSite struct {
	robots     string
	lock       sync.Mutex
	requests   []string
	userAgents map[string]bool
	hooliFixed bool
}

func (site *fakeSite) start(t *testing.T) string {
	t.Helper()
	pages := map[string]string{
		"/companies/acme":    makeCompanyPage("https://acme.example/", "https://jobs.ashbyhq.com/Acme"),
		"/companies/globex":  makeCompanyPage("https://globex.example/", "https://globex.example/careers", "https://job-boards.greenhouse.io/globex/jobs/42"),
		"/companies/initech": makeCompanyPage("https://initech.example/", "https://initech.example/careers"),
		"/companies/hooli":   makeCompanyPage("https://hooli.example/", "https://jobs.lever.co/hooli"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site.lock.Lock()
		defer site.lock.Unlock()
		site.requests = append(site.requests, r.URL.Path)
		site.userAgents[r.UserAgent()] = true
		switch {
		case r.URL.Path == "/robots.txt":
			io.WriteString(w, site.robots)
		case r.URL.Path == listPath:
			io.WriteString(w, listPage)
		case r.URL.Path == "/companies/hooli" && !site.hooliFixed:
			http.Error(w, "busy", http.StatusServiceUnavailable)
		case pages[r.URL.Path] != "":
			io.WriteString(w, pages[r.URL.Path])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func (site *fakeSite) takeRequests() []string {
	site.lock.Lock()
	defer site.lock.Unlock()
	requests := site.requests
	site.requests = nil
	return requests
}

// fakeBoards answers each board's titles; an unknown board has none.
type fakeBoards struct {
	titles map[string][]string
	asked  []string
}

func (boards *fakeBoards) ListPostingTitles(_ context.Context, provider, boardToken string) ([]string, error) {
	boards.asked = append(boards.asked, provider+"/"+boardToken)
	titles, known := boards.titles[provider+"/"+boardToken]
	if !known {
		return nil, jobboards.ErrPostingAPIOff
	}
	return titles, nil
}

func startReader(t *testing.T, robots string) (*Reader, *store.Store, *fakeSite, *fakeBoards, *time.Time) {
	t.Helper()
	hub := store.New(testdatabase.New(t))
	if _, err := hub.SaveJobCriteria(context.Background(), owner, store.JobCriteria{Roles: []string{"Frontend Engineer"}}); err != nil {
		t.Fatal(err)
	}
	site := &fakeSite{robots: robots, userAgents: map[string]bool{}}
	boards := &fakeBoards{titles: map[string][]string{
		jobboards.Ashby + "/acme":        {"Accountant", "Senior Frontend Engineer"},
		jobboards.Greenhouse + "/globex": {"Accountant"},
		jobboards.Lever + "/hooli":       {"Frontend Engineer"},
	}}
	now := time.Now()
	reader := NewReader(hub, boards)
	reader.SiteBase, reader.RequestPause, reader.Now = site.start(t), 0, func() time.Time { return now }
	return reader, hub, site, boards, &now
}

func TestAWeeklyPassReadsRobotsFirstAndKeepsTheBoardsThatNameARole(t *testing.T) {
	reader, hub, site, boards, now := startReader(t, "User-agent: *\nAllow: /\n")
	ctx := context.Background()

	summary, err := reader.ReadOnce(ctx)
	if err != nil || summary != (PassSummary{Listed: 4, New: 4, PagesRead: 3, Checked: 2, Kept: 1, Failed: 1}) {
		t.Fatalf("summary = %+v, %v; want four listed, hooli's page failed, and acme's board kept but not globex's", summary, err)
	}
	if requests := site.takeRequests(); len(requests) != 6 || requests[0] != "/robots.txt" || requests[1] != listPath {
		t.Errorf("requests = %v; want robots.txt first, then the list and the four pages", requests)
	}
	if len(site.userAgents) != 1 || !site.userAgents[UserAgent] {
		t.Errorf("user agents = %v; want every request to name the hub", site.userAgents)
	}
	acme, err := hub.GetJobBoardByToken(ctx, jobboards.Ashby, "acme")
	if err != nil || acme.CompanyName != "Acme" || acme.FoundBy != store.JobBoardFoundByDiscovery || acme.CompanyID != nil {
		t.Errorf("acme's board = %+v, %v; want it kept under the list's name, for the poller to read", acme, err)
	}
	suggested, err := hub.ListGalleryCompaniesNotInHub(ctx)
	if err != nil || len(suggested) != 1 || suggested[0].Name != "Acme" || suggested[0].Website != "https://acme.example/" ||
		suggested[0].CareersURL != "https://jobs.ashbyhq.com/Acme" {
		t.Errorf("suggestable = %+v, %v; want only Acme, with its site and careers link", suggested, err)
	}

	asked := len(boards.asked)
	if summary, err := reader.ReadOnce(ctx); err != nil || summary != (PassSummary{}) {
		t.Errorf("a second pass the same week = %+v, %v; want nothing read", summary, err)
	}
	if requests := site.takeRequests(); len(requests) != 0 || len(boards.asked) != asked {
		t.Errorf("a second pass the same week asked the site %v and %d boards", requests, len(boards.asked)-asked)
	}

	*now = now.Add(8 * 24 * time.Hour)
	site.hooliFixed = true
	boards.titles[jobboards.Greenhouse+"/globex"] = []string{"Frontend Engineer"}
	summary, err = reader.ReadOnce(ctx)
	if err != nil || summary != (PassSummary{Listed: 4, PagesRead: 1, Checked: 2, Kept: 2}) {
		t.Errorf("next week's summary = %+v, %v; want hooli's page read, and globex's board checked again and kept", summary, err)
	}
	if requests := site.takeRequests(); fmt.Sprint(requests) != fmt.Sprint([]string{"/robots.txt", listPath, "/companies/hooli"}) {
		t.Errorf("next week's requests = %v; want no page read twice", requests)
	}
}

func TestRobotsThatDisallowTheListOrThePagesAreObeyed(t *testing.T) {
	reader, hub, site, _, _ := startReader(t, "User-agent: *\nDisallow: /\n")
	ctx := context.Background()
	summary, err := reader.ReadOnce(ctx)
	if err != nil || summary != (PassSummary{Disallowed: true}) {
		t.Errorf("summary = %+v, %v; want the pass to stop at robots.txt", summary, err)
	}
	if requests := site.takeRequests(); fmt.Sprint(requests) != "[/robots.txt]" {
		t.Errorf("requests = %v; want robots.txt only", requests)
	}
	if lastRead, err := hub.GetLastGalleryListRead(ctx); err != nil || lastRead == nil {
		t.Errorf("last read = %v, %v; a disallowed pass still waits a week", lastRead, err)
	}

	reader, _, site, _, _ = startReader(t, "User-agent: *\nDisallow: /companies/\n")
	summary, err = reader.ReadOnce(ctx)
	if err != nil || summary.Listed != 4 || summary.PagesRead != 0 || summary.PagesDisallowed != 4 {
		t.Errorf("summary = %+v, %v; want the list read and no page", summary, err)
	}
	if requests := site.takeRequests(); fmt.Sprint(requests) != fmt.Sprint([]string{"/robots.txt", listPath}) {
		t.Errorf("requests = %v", requests)
	}
}

func TestOnlyTheNameSiteAndCareersLinkAreRead(t *testing.T) {
	listed := readListedCompanies([]byte(listPage))
	if fmt.Sprint(listed) != fmt.Sprint([]store.GalleryCompany{{Slug: "acme", Name: "Acme"}, {Slug: "globex", Name: "Globex & Co"},
		{Slug: "initech", Name: "Initech"}, {Slug: "hooli", Name: "Hooli"}}) {
		t.Errorf("listed = %+v", listed)
	}
	page := readCompanyPage([]byte(makeCompanyPage("https://globex.example/", "https://globex.example/careers", "https://job-boards.greenhouse.io/globex/jobs/42")))
	if page != (store.GalleryPage{Website: "https://globex.example/", CareersURL: "https://globex.example/careers", Provider: jobboards.Greenhouse, BoardToken: "globex"}) {
		t.Errorf("page = %+v; want the board read from a posting when the careers link is the company's own", page)
	}
}
