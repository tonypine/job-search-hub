package boardpoller_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/boardpoller"
	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// fakeBoards answers per board token: "working" has two postings, "broken"
// fails, "pageonly" has its posting API off. It records which boards were read.
type fakeBoards struct{ fetched []string }

func (boards *fakeBoards) FetchPostings(_ context.Context, _, boardToken string) ([]store.JobPosting, error) {
	boards.fetched = append(boards.fetched, boardToken)
	switch boardToken {
	case "working":
		return []store.JobPosting{
			{ExternalID: "1", Title: "Frontend Engineer", URL: "https://jobs.example/1"},
			{ExternalID: "2", Title: "Backend Engineer", URL: "https://jobs.example/2"},
		}, nil
	case "pageonly":
		return nil, jobboards.ErrPostingAPIOff
	case "unwatched":
		return []store.JobPosting{
			{ExternalID: "3", Title: "Frontend Engineer", URL: "https://jobs.example/3"},
			{ExternalID: "4", Title: "Sales Manager", URL: "https://jobs.example/4"},
			{ExternalID: "6", Title: "Distributed Systems Engineer", URL: "https://jobs.example/6"},
			{ExternalID: "5", Title: "Senior Frontend Engineer", Location: "Berlin, must be based in the EU", URL: "https://jobs.example/5"},
		}, nil
	default:
		return nil, errors.New("provider answered 503")
	}
}

func addCompanyWithBoard(t *testing.T, hub *store.Store, domain, boardToken string, watched bool) {
	t.Helper()
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: domain, Domain: domain})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: company.ID, Provider: "ashby", BoardToken: boardToken, Verified: true}); err != nil {
		t.Fatal(err)
	}
	if watched {
		if _, _, err := hub.AddToWatchList(ctx, owner, company.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAPollSyncsEveryVerifiedBoardAndCarriesOnPastFailures(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	if _, err := hub.SaveJobCriteria(context.Background(), owner, store.JobCriteria{
		Roles: []string{"Frontend Engineer"}, ExcludedRoleTerms: []string{"Sales"}, IneligibleLocationTerms: []string{"must be based in the EU"},
	}); err != nil {
		t.Fatal(err)
	}
	addCompanyWithBoard(t, hub, "broken.com", "broken", true)
	addCompanyWithBoard(t, hub, "working.com", "working", true)
	addCompanyWithBoard(t, hub, "pageonly.com", "pageonly", true)
	addCompanyWithBoard(t, hub, "unwatched.com", "unwatched", false)
	boards := &fakeBoards{}

	summary, err := boardpoller.New(hub, boards).PollOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Boards != 4 || summary.Failed != 1 || summary.Skipped != 1 || summary.Totals.Created != 3 || summary.Totals.Dropped != 3 {
		t.Fatalf("summary = %+v; want the watched board's two postings and the unwatched board's one that could fit", summary)
	}
	var jobs int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM jobs`).Scan(&jobs); err != nil || jobs != 3 {
		t.Fatalf("jobs = %d, err = %v", jobs, err)
	}

	again, err := boardpoller.New(hub, boards).PollOnce(context.Background())
	if err != nil || again.Totals.Created != 0 || again.Totals.Closed != 0 {
		t.Fatalf("second poll = %+v, %v; want nothing new and nothing closed", again, err)
	}
}
