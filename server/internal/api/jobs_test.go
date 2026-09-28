package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
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

// stubRates price one BRL at 0.2 USD.
type stubRates struct{}

func (stubRates) GetRates(_ context.Context, base string) (map[string]float64, error) {
	if base != "BRL" {
		return nil, errors.New("no rates for " + base)
	}
	return map[string]float64{"USD": 0.2}, nil
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

type jobDetailsAnswer struct {
	Job         store.Job               `json:"job"`
	CompanyName *string                 `json:"company_name"`
	Facts       *store.LabelledJobFacts `json:"facts"`
	Application *store.Application      `json:"application"`
	Phase       *store.PipelinePhase    `json:"phase"`
}

func readJobDetails(t *testing.T, service apiUnderTest, id string) (int, jobDetailsAnswer) {
	t.Helper()
	status, body := send(t, http.MethodGet, service.url+"/v1/jobs/"+id, ownerToken, "")
	var details jobDetailsAnswer
	if status == http.StatusOK {
		if err := json.Unmarshal(body, &details); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
	}
	return status, details
}

func TestAJobsDetailsCarryItsFactsAndItsPhase(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	read, _, _ := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &company.ID, Title: "Backend Engineer", URL: "https://acme.com/jobs/1", Description: "Go."})
	unread, _, _ := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Designer", URL: "https://other.com/jobs/2"})

	prompt, _ := service.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	awaiting, _ := service.hub.ListJobsAwaitingFacts(ctx, prompt.ID, 10)
	for _, job := range awaiting {
		if job.ID == read.ID {
			if err := service.hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: job.ID, PromptID: prompt.ID, Model: "test-model", TextHash: job.TextHash, Facts: json.RawMessage(`{"technologies":["Go"]}`)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, _, err := service.hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &read.ID}); err != nil {
		t.Fatal(err)
	}

	status, details := readJobDetails(t, service, read.ID.String())
	if status != http.StatusOK || details.Job.Description != "Go." || details.CompanyName == nil || *details.CompanyName != "Acme" {
		t.Fatalf("details = %d %+v", status, details)
	}
	if details.Facts == nil || len(details.Facts.Entries) != 1 || details.Facts.Model != "test-model" || details.Facts.PromptVersion != 1 {
		t.Fatalf("facts = %+v", details.Facts)
	}
	entry := details.Facts.Entries[0]
	var technologies []string
	if err := json.Unmarshal(entry.Value, &technologies); err != nil || entry.Key != "technologies" || entry.Title != "Technologies" ||
		entry.Description == "" || len(technologies) != 1 || technologies[0] != "Go" {
		t.Fatalf("entry = %+v", entry)
	}
	if details.Application == nil || details.Phase == nil || details.Phase.Name != "Saved" || details.Application.PhaseID != details.Phase.ID {
		t.Fatalf("application = %+v, phase = %+v", details.Application, details.Phase)
	}

	if status, bare := readJobDetails(t, service, unread.ID.String()); status != http.StatusOK || bare.Facts != nil || bare.Application != nil || bare.Phase != nil || bare.CompanyName != nil {
		t.Fatalf("unread job = %d %+v", status, bare)
	}
	for _, id := range []string{"7c9e6679-7425-40de-944b-e07fc1f90ae7", "not-an-id"} {
		if status, _ := readJobDetails(t, service, id); status != http.StatusNotFound {
			t.Errorf("job %s: %d, want 404", id, status)
		}
	}
}

func TestTheJobsListAndDetailsCarryTheFit(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := service.hub.SaveJobCriteria(ctx, owner, store.JobCriteria{
		Technologies: []string{"Go"}, SeniorityLevels: []string{"Senior"}, EligibleLocationTerms: []string{"Americas"},
	}); err != nil {
		t.Fatal(err)
	}
	job, _, _ := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Senior Go Engineer", URL: "https://acme.com/jobs/9", Location: "Americas"})

	status, body := send(t, http.MethodGet, service.url+"/v1/jobs", ownerToken, "")
	var list struct {
		Jobs []struct {
			Job store.Job  `json:"job"`
			Fit jobfit.Fit `json:"fit"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &list); status != http.StatusOK || err != nil || len(list.Jobs) != 1 {
		t.Fatalf("list: %d %s", status, body)
	}
	if fit := list.Jobs[0].Fit; fit.Level != jobfit.LevelGood || len(fit.Checks) != 3 {
		t.Fatalf("list fit = %+v", fit)
	}

	status, body = send(t, http.MethodGet, service.url+"/v1/jobs/"+job.ID.String(), ownerToken, "")
	var details struct {
		Fit jobfit.Fit `json:"fit"`
	}
	if err := json.Unmarshal(body, &details); status != http.StatusOK || err != nil || details.Fit.Level != jobfit.LevelGood {
		t.Fatalf("details: %d %s", status, body)
	}
}

func TestTheFitJudgesPayInTheTakeHomeCurrency(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	hiring := store.HiringTakeHome{Share: 0.84, PaymentsPerYear: 12}
	if _, err := service.hub.SaveJobCriteria(ctx, owner, store.JobCriteria{TakeHome: &store.TakeHome{
		Currency: "BRL", MinimumMonthly: 16000, TargetMonthly: 44000, CLT: hiring, PJ: hiring, ForeignContractor: hiring,
	}}); err != nil {
		t.Fatal(err)
	}
	company, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	board, _ := service.hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: company.ID, Provider: "lever", BoardToken: "acme", Verified: true})
	posting := store.JobPosting{ExternalID: "1", Title: "Engineer", URL: "https://jobs.lever.co/acme/1"}
	posting.Pay = &store.Pay{Ranges: []store.PayRange{{Min: 60000, Max: 60000, Currency: "USD", Interval: "year"}}}
	if _, err := service.hub.SyncBoardJobs(ctx, owner, board, []store.JobPosting{posting}, time.Now()); err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/jobs", ownerToken, "")
	var list struct {
		Jobs []struct {
			Fit jobfit.Fit `json:"fit"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &list); status != http.StatusOK || err != nil || len(list.Jobs) != 1 {
		t.Fatalf("list: %d %s", status, body)
	}
	var payCheck *jobfit.Check
	for index := range list.Jobs[0].Fit.Checks {
		if list.Jobs[0].Fit.Checks[index].Name == "Pay" {
			payCheck = &list.Jobs[0].Fit.Checks[index]
		}
	}
	if payCheck == nil || payCheck.Verdict != jobfit.VerdictYes || payCheck.Reason != "about BRL 21.0k a month take-home, 48% of the target" {
		t.Fatalf("pay check = %+v", payCheck)
	}
}
