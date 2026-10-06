package boarddiscovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// fakeBoards answers each board's titles; an unknown board has none.
type fakeBoards struct {
	titles map[string][]string
	lock   sync.Mutex
	asked  []string
}

func (boards *fakeBoards) ListPostingTitles(_ context.Context, provider, boardToken string) ([]string, error) {
	boards.lock.Lock()
	defer boards.lock.Unlock()
	boards.asked = append(boards.asked, provider+"/"+boardToken)
	titles, known := boards.titles[provider+"/"+boardToken]
	if !known {
		return nil, jobboards.ErrPostingAPIOff
	}
	return titles, nil
}

func (boards *fakeBoards) FetchBoardCompanyName(_ context.Context, provider, boardToken string) (string, error) {
	if provider == jobboards.Greenhouse && boardToken == "acme" {
		return "Acme Inc.", nil
	}
	return "", nil
}

// startIndex serves a crawl whose Greenhouse index has one page and whose
// Lever index has one; the first page count it's asked for times out.
func startIndex(t *testing.T) (string, *int) {
	t.Helper()
	var server *httptest.Server
	pageReads := 0
	timedOut := false
	routes := http.NewServeMux()
	routes.HandleFunc("GET /collinfo.json", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `[{"id":"CC-MAIN-2026-39","cdx-api":"%s/CC-MAIN-2026-39-index"},{"id":"CC-MAIN-2026-34"}]`, server.URL)
	})
	routes.HandleFunc("GET /CC-MAIN-2026-39-index", func(w http.ResponseWriter, r *http.Request) {
		pattern := r.URL.Query().Get("url")
		if r.URL.Query().Get("showNumPages") == "true" {
			if !timedOut {
				timedOut = true
				http.Error(w, "<html>504 Gateway Time-out</html>", http.StatusGatewayTimeout)
				return
			}
			pages := 0
			if pattern == "boards.greenhouse.io/*" || pattern == "jobs.lever.co/*" {
				pages = 1
			}
			fmt.Fprintf(w, `{"pages": %d, "pageSize": 5, "blocks": 5}`, pages)
			return
		}
		pageReads++
		switch pattern {
		case "boards.greenhouse.io/*":
			io.WriteString(w, `{"url": "https://boards.greenhouse.io/acme/jobs/1"}
{"url": "https://boards.greenhouse.io/acme"}
{"url": "https://boards.greenhouse.io/embed/job_board?for=initech"}
{"url": "https://boards.greenhouse.io/globex"}
{"url": "https://boards.greenhouse.io/%E2%9C%93"}
`)
		case "jobs.lever.co/*":
			fmt.Fprint(w, `{"url": "https://jobs.lever.co/hooli-labs/abc"}`+"\n")
		}
	})
	server = httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return server.URL, &pageReads
}

func TestAPassReadsTheIndexAndKeepsTheBoardsThatNameARole(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveJobCriteria(ctx, owner, store.JobCriteria{Roles: []string{"Frontend Engineer"}}); err != nil {
		t.Fatal(err)
	}
	boards := &fakeBoards{titles: map[string][]string{
		jobboards.Greenhouse + "/acme":    {"Accountant", "Senior Frontend Engineer"},
		jobboards.Greenhouse + "/globex":  {"Accountant"},
		jobboards.Greenhouse + "/initech": {"Office Manager"},
		jobboards.Lever + "/hooli-labs":   {"Frontend Engineer"},
	}}
	indexBase, pageReads := startIndex(t)
	discoverer := New(hub, boards)
	discoverer.IndexBase, discoverer.RequestPause, discoverer.IndexRetryPause = indexBase, 0, 0

	summary, err := discoverer.DiscoverOnce(ctx)
	if err != nil || summary != (PassSummary{IndexPagesRead: 2, TokensFound: 4, Checked: 4, Kept: 2}) {
		t.Fatalf("summary = %+v, %v; want acme, initech, globex and hooli-labs found, and acme and hooli-labs kept", summary, err)
	}
	acme, err := hub.GetJobBoardByToken(ctx, jobboards.Greenhouse, "acme")
	if err != nil || acme.CompanyName != "Acme Inc." || acme.FoundBy != store.JobBoardFoundByDiscovery || acme.VerifiedAt == nil {
		t.Errorf("acme = %+v, %v", acme, err)
	}
	if hooli, err := hub.GetJobBoardByToken(ctx, jobboards.Lever, "hooli-labs"); err != nil || hooli.CompanyName != "Hooli Labs" {
		t.Errorf("hooli-labs = %+v, %v; want its name made from its token", hooli, err)
	}

	asked, reads := len(boards.asked), *pageReads
	if summary, err := discoverer.DiscoverOnce(ctx); err != nil || summary != (PassSummary{}) {
		t.Errorf("second pass = %+v, %v; want nothing read or checked again", summary, err)
	}
	if len(boards.asked) != asked || *pageReads != reads {
		t.Errorf("the second pass asked %d boards and read %d pages more", len(boards.asked)-asked, *pageReads-reads)
	}
}

func TestBoardTokensAreReadFromEachProvidersURLs(t *testing.T) {
	page := []byte(strings.Join([]string{
		`{"url": "https://acme.recruitee.com/o/frontend-engineer"}`,
		`{"url": "https://www.recruitee.com/pricing"}`,
		`{"url": "https://acme.recruitee.com/"}`,
		`{"url": "https://careers.globex.recruitee.com/"}`,
	}, "\n"))
	got := readBoardTokens(page, boardIndex{provider: jobboards.Recruitee, getToken: getRecruiteeToken})
	if fmt.Sprint(got) != "[acme]" {
		t.Errorf("tokens = %v, want [acme]", got)
	}
}
