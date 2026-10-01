package boardfinder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

type noRates struct{}

func (noRates) GetRates(context.Context, string) (map[string]float64, error) { return nil, nil }

// fakeBoards answers for the boards it knows; any other board is unknown.
type fakeBoards struct {
	titles  map[string][]string
	failing map[string]bool
	// limiting are providers that answer 429 to every request.
	limiting map[string]bool
	lock     sync.Mutex
	asked    []string
}

func (boards *fakeBoards) ListPostingTitles(_ context.Context, provider, boardToken string) ([]string, error) {
	key := provider + "/" + boardToken
	boards.lock.Lock()
	boards.asked = append(boards.asked, key)
	boards.lock.Unlock()
	if boards.failing[boardToken] {
		return nil, errors.New("greenhouse answered 500")
	}
	if boards.limiting[provider] {
		return nil, fmt.Errorf("%w: %s answered 429", jobboards.ErrRateLimited, provider)
	}
	titles, known := boards.titles[key]
	if !known {
		return nil, jobboards.ErrPostingAPIOff
	}
	return titles, nil
}

func (boards *fakeBoards) countAsked(boardToken string) int {
	count := 0
	for _, key := range boards.asked {
		if key == jobboards.Lever+"/"+boardToken {
			count++
		}
	}
	return count
}

func TestAPassFindsTheBoardThatListsAFeedJobsTitle(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveJobCriteria(ctx, owner, store.JobCriteria{Roles: []string{"Frontend Engineer"}, SeniorityLevels: []string{"Senior"}}); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	feedJob := func(company, title, slug string) store.JobPosting {
		return store.JobPosting{ExternalID: company + title, CompanyName: company, Title: title, Location: "Remote", ExpiresAt: &expiresAt,
			URL: "https://himalayas.app/companies/" + slug + "/jobs/" + slug, Description: "Build."}
	}
	if _, err := hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, []store.JobPosting{
		feedJob("Acme Labs", "Senior Frontend Engineer", "acme-hq"),
		feedJob("Namesake", "Frontend Engineer", "namesake"),
		feedJob("Flaky Inc.", "Frontend Engineer", "flaky"),
		feedJob("Sales Corp", "Sales Manager", "sales-corp"),
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	boards := &fakeBoards{
		titles: map[string][]string{
			jobboards.Lever + "/acmelabs":      {"Office Manager", "senior frontend engineer"},
			jobboards.Recruitee + "/acmelabs":  {"Senior Frontend Engineer"},
			jobboards.Greenhouse + "/namesake": {"Accountant"},
		},
		failing: map[string]bool{"flaky": true},
	}
	finder := New(hub, boards, noRates{})
	finder.RequestPause = 0

	summary, err := finder.FindOnce(ctx)
	if err != nil || summary != (PassSummary{Searched: 1, Found: 1, Unfinished: 1}) {
		t.Fatalf("summary = %+v, %v; want Acme Labs found, Namesake searched, Flaky Inc. unfinished and Sales Corp left out", summary, err)
	}
	board, err := hub.GetJobBoardByToken(ctx, jobboards.Lever, "acmelabs")
	if err != nil || board.CompanyID != nil || board.CompanyName != "Acme Labs" || board.VerifiedAt == nil ||
		board.BoardURL != "https://jobs.lever.co/acmelabs" || board.OpenPostingCount == nil || *board.OpenPostingCount != 2 {
		t.Fatalf("board = %+v, %v", board, err)
	}
	if _, err := hub.GetJobBoardByToken(ctx, jobboards.Greenhouse, "namesake"); !errors.Is(err, store.ErrJobBoardNotFound) {
		t.Errorf("Namesake's board was stored though it lists none of its titles: %v", err)
	}

	flakyAsked := boards.countAsked("flaky")
	if summary, err := finder.FindOnce(ctx); err != nil || summary != (PassSummary{Unfinished: 1}) {
		t.Errorf("second pass = %+v, %v; only the unfinished search should repeat", summary, err)
	}
	if boards.countAsked("flaky") != flakyAsked+1 {
		t.Errorf("the failed search wasn't repeated")
	}

	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme Labs", Domain: "acmelabs.example"})
	if err != nil {
		t.Fatal(err)
	}
	board, err = hub.GetJobBoardByToken(ctx, jobboards.Lever, "acmelabs")
	if err != nil || board.CompanyID == nil || *board.CompanyID != company.ID {
		t.Errorf("after the company was added, board = %+v, %v; want it tied to %s", board, err, company.ID)
	}
}

func TestBoardTokenCandidatesComeFromTheNameAndTheHimalayasSlug(t *testing.T) {
	got := getBoardTokenCandidates(FeedCompany{Name: "Teravision Technologies, Inc.", HimalayasSlug: "teravision-tech"})
	want := []string{"teravisiontechnologies", "teravision-technologies", "teravision-tech", "teravision"}
	if len(got) != len(want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("candidates = %v, want %v", got, want)
		}
	}
}

func TestAProviderLimitingRequestsIsLeftAloneForThePass(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveJobCriteria(ctx, owner, store.JobCriteria{Roles: []string{"Frontend Engineer"}}); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	var postings []store.JobPosting
	for _, company := range []string{"Alpha", "Beta", "Gamma"} {
		postings = append(postings, store.JobPosting{ExternalID: company, CompanyName: company, Title: "Frontend Engineer", Location: "Remote",
			ExpiresAt: &expiresAt, URL: "https://himalayas.app/companies/" + strings.ToLower(company) + "/jobs/x", Description: "Build."})
	}
	if _, err := hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, postings, time.Now()); err != nil {
		t.Fatal(err)
	}
	boards := &fakeBoards{limiting: map[string]bool{jobboards.Workable: true}}
	finder := New(hub, boards, noRates{})
	finder.RequestPause = 0

	summary, err := finder.FindOnce(ctx)
	if err != nil || summary != (PassSummary{Unfinished: 3}) {
		t.Fatalf("summary = %+v, %v; want every search unfinished, to be resumed", summary, err)
	}
	workableAsked := 0
	for _, key := range boards.asked {
		if strings.HasPrefix(key, jobboards.Workable+"/") {
			workableAsked++
		}
	}
	if workableAsked != 1 {
		t.Errorf("Workable was asked %d times after answering 429; want once", workableAsked)
	}

	boards.limiting, boards.asked = nil, nil
	summary, err = finder.FindOnce(ctx)
	if err != nil || summary != (PassSummary{Searched: 3}) {
		t.Fatalf("next pass = %+v, %v; want the three searches finished", summary, err)
	}
	for _, key := range boards.asked {
		if !strings.HasPrefix(key, jobboards.Workable+"/") {
			t.Errorf("the next pass asked %s again; only Workable was left", key)
		}
	}
}
