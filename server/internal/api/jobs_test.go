package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// stubPostings serves one Greenhouse posting on each of the boards "acme"
// and "stranger"; every other posting fails to fetch.
type stubPostings struct{}

func (stubPostings) FetchPosting(_ context.Context, reference jobboards.PostingReference) (store.JobPosting, error) {
	if reference.Provider == "greenhouse" && reference.PostingID == "7" {
		return store.JobPosting{
			ExternalID: "7", Title: "Frontend Engineer", Location: "Remote (Americas)",
			URL: "https://job-boards.greenhouse.io/" + reference.BoardToken + "/jobs/7", Description: "React.",
		}, nil
	}
	return store.JobPosting{}, errors.New("no such posting")
}

type addedJob struct {
	Job     store.Job `json:"job"`
	Created bool      `json:"created"`
}

func TestAddingAGreenhouseURLJoinsTheStoredBoardsJobs(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	board, err := service.hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: company.ID, Provider: "greenhouse", BoardToken: "acme", Verified: true})
	if err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodPost, service.url+"/v1/jobs", ownerToken, `{"url":"https://job-boards.greenhouse.io/acme/jobs/7"}`)
	var added addedJob
	if err := json.Unmarshal(body, &added); status != http.StatusCreated || err != nil {
		t.Fatalf("add: %d %s", status, body)
	}
	if added.Job.Title != "Frontend Engineer" || added.Job.CompanyID == nil || *added.Job.CompanyID != company.ID ||
		added.Job.Source != store.JobSourceJobBoard || added.Job.JobBoardID == nil || *added.Job.JobBoardID != board.ID {
		t.Fatalf("job = %+v", added.Job)
	}

	// The board's next sync sees the same posting and keeps one job.
	result, err := service.hub.SyncBoardJobs(ctx, store.Actor{Kind: store.ActorSystem}, board,
		[]store.JobPosting{{ExternalID: "7", Title: "Frontend Engineer", URL: added.Job.URL}}, time.Now())
	if err != nil || result.Created != 0 {
		t.Fatalf("sync after add = %+v, %v; want no new job", result, err)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/jobs", ownerToken, `{"url":"https://job-boards.greenhouse.io/acme/jobs/7"}`); status != http.StatusOK {
		t.Fatalf("adding again: %d, want 200", status)
	}
}

func TestAddingURLsOfUnknownBoardsAndSites(t *testing.T) {
	service := startAPI(t)

	status, body := send(t, http.MethodPost, service.url+"/v1/jobs", ownerToken, `{"url":"https://job-boards.greenhouse.io/stranger/jobs/7"}`)
	var fromUnknownBoard addedJob
	if err := json.Unmarshal(body, &fromUnknownBoard); status != http.StatusCreated || err != nil || fromUnknownBoard.Job.Title != "Frontend Engineer" ||
		fromUnknownBoard.Job.Source != store.JobSourceManual || fromUnknownBoard.Job.CompanyID != nil {
		t.Fatalf("unknown board: %d %s", status, body)
	}

	status, body = send(t, http.MethodPost, service.url+"/v1/jobs", ownerToken, `{"url":"https://acme.com/careers/42","title":"Staff Engineer"}`)
	var fromSite addedJob
	if err := json.Unmarshal(body, &fromSite); status != http.StatusCreated || err != nil || fromSite.Job.Title != "Staff Engineer" {
		t.Fatalf("site URL: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/jobs", ownerToken, `{"url":"https://acme.com/careers/43"}`); status != http.StatusBadRequest {
		t.Fatalf("site URL without a title: %d, want 400", status)
	}
}

func TestTheJobsListFiltersBySearchCompanyAndStatus(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	acme, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	board, _ := service.hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: acme.ID, Provider: "lever", BoardToken: "acme", Verified: true})
	system := store.Actor{Kind: store.ActorSystem}
	if _, err := service.hub.SyncBoardJobs(ctx, system, board, []store.JobPosting{
		{ExternalID: "1", Title: "Frontend Engineer", Location: "Remote (Americas)", URL: "https://jobs.lever.co/acme/1"},
		{ExternalID: "2", Title: "Data Analyst", Location: "Toronto", URL: "https://jobs.lever.co/acme/2"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.hub.SyncBoardJobs(ctx, system, board, []store.JobPosting{
		{ExternalID: "1", Title: "Frontend Engineer", Location: "Remote (Americas)", URL: "https://jobs.lever.co/acme/1"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Staff Engineer", URL: "https://other.com/jobs/9"}); err != nil {
		t.Fatal(err)
	}

	for query, want := range map[string]int{
		"":                                2,
		"?query=americas":                 1,
		"?query=ACME":                     1,
		"?status=closed":                  1,
		"?status=all":                     3,
		"?company_id=" + acme.ID.String(): 1,
		"?limit=1":                        2,
	} {
		status, body := send(t, http.MethodGet, service.url+"/v1/jobs"+query, ownerToken, "")
		var listed struct {
			Jobs  []store.JobListItem `json:"jobs"`
			Total int                 `json:"total"`
		}
		if err := json.Unmarshal(body, &listed); status != http.StatusOK || err != nil || listed.Total != want {
			t.Errorf("GET /v1/jobs%s: %d total=%d, want %d (%s)", query, status, listed.Total, want, body)
		}
	}
}
