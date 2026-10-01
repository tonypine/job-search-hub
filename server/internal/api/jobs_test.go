package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

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
	Brief     *store.JobBrief `json:"brief"`
	ScreenOut []struct {
		Name, Verdict, Answer, Evidence string
	} `json:"screen_out"`
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
	factsPrompt, _ := service.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	if err := service.hub.SaveJobFacts(ctx, store.NewJobFacts{
		JobID: job.ID, PromptID: factsPrompt.ID, Model: "test-model", TextHash: []byte{1}, Facts: json.RawMessage(`{"years_of_experience":6}`),
	}); err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/jobs", ownerToken, "")
	var list struct {
		Jobs []struct {
			Job   store.Job       `json:"job"`
			Fit   jobfit.Fit      `json:"fit"`
			Facts json.RawMessage `json:"facts"`
		} `json:"jobs"`
		FactColumns []store.JobFactColumn `json:"fact_columns"`
	}
	if err := json.Unmarshal(body, &list); status != http.StatusOK || err != nil || len(list.Jobs) != 1 {
		t.Fatalf("list: %d %s", status, body)
	}
	if fit := list.Jobs[0].Fit; fit.Level != jobfit.LevelGood || len(fit.Checks) != 4 {
		t.Fatalf("list fit = %+v", fit)
	}
	if !strings.Contains(string(list.Jobs[0].Facts), `"years_of_experience":6`) || len(list.FactColumns) == 0 ||
		list.FactColumns[0] != (store.JobFactColumn{Key: "summary", Title: "Summary"}) {
		t.Fatalf("list facts = %s, columns = %+v", list.Jobs[0].Facts, list.FactColumns)
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

func TestJobsAreDismissedAndRestoredTogether(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	board, _ := service.hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: company.ID, Provider: "lever", BoardToken: "acme", Verified: true})
	postings := []store.JobPosting{{ExternalID: "1", Title: "One", URL: "https://example.com/1"}, {ExternalID: "2", Title: "Two", URL: "https://example.com/2"}}
	if _, err := service.hub.SyncBoardJobs(ctx, store.Actor{Kind: store.ActorSystem}, board, postings, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := service.hub.ListJobs(ctx, store.JobFilter{})
	ids := `["` + jobs[0].Job.ID.String() + `","` + jobs[1].Job.ID.String() + `"]`

	status, body := send(t, http.MethodPost, service.url+"/v1/jobs/dismiss", ownerToken, `{"job_ids":`+ids+`,"reason":"agency"}`)
	var dismissed struct{ Jobs []store.Job }
	if err := json.Unmarshal(body, &dismissed); status != http.StatusOK || err != nil || len(dismissed.Jobs) != 2 {
		t.Fatalf("dismiss: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/jobs?status=dismissed", ownerToken, "")
	var listed struct{ Total int }
	if err := json.Unmarshal(body, &listed); status != http.StatusOK || err != nil || listed.Total != 2 {
		t.Fatalf("dismissed list: %d %s", status, body)
	}
	if status, body := send(t, http.MethodPost, service.url+"/v1/jobs/restore", ownerToken, `{"job_ids":`+ids+`}`); status != http.StatusOK {
		t.Fatalf("restore: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/jobs/dismiss", ownerToken, `{"job_ids":["`+store.Job{}.ID.String()+`"]}`); status != http.StatusNotFound {
		t.Errorf("an unknown job: %d, want 404", status)
	}
}

func TestAJobsDetailsCarryItsBriefAndScreenOutAnswers(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	job, _, _ := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Senior Front-End Engineer", URL: "https://acme.com/jobs/1", Description: "React."})
	prompt, _ := service.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	awaiting, _ := service.hub.ListJobsAwaitingFacts(ctx, prompt.ID, 10)
	facts := `{"location":{"evidence":"Remote in LATAM","restriction":"LATAM","open_to_brazil":"yes","reason":"LATAM"},
		"seniority":{"evidence":"Senior engineer","as_written":"Senior","levels":["senior"]},
		"contract":{"evidence":"as a contractor","as_written":"contractor","kinds":["contractor"]}}`
	if err := service.hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: job.ID, PromptID: prompt.ID, Model: "m", TextHash: awaiting[0].TextHash, Facts: json.RawMessage(facts)}); err != nil {
		t.Fatal(err)
	}
	hash, _ := service.hub.GetKnowledgeHash(ctx)
	if err := service.hub.SaveJobBrief(ctx, store.JobBrief{JobID: job.ID, Tier: store.JobBriefTierPre, PromptID: prompt.ID, Model: "local",
		Match: "strong", Reason: "React at scale.", KnowledgeHash: hash}); err != nil {
		t.Fatal(err)
	}

	_, details := readJobDetails(t, service, job.ID.String())
	if details.Brief == nil || details.Brief.Match != "strong" || details.Brief.Reason != "React at scale." {
		t.Fatalf("brief = %+v", details.Brief)
	}
	answers := map[string]string{}
	for _, answer := range details.ScreenOut {
		answers[answer.Name] = answer.Verdict + "|" + answer.Answer + "|" + answer.Evidence
	}
	want := map[string]string{
		"Hires from Brazil": "yes|open to someone in Brazil|Remote in LATAM",
		"Level":             "unclear|the criteria name no levels|Senior engineer",
		"Timezone":          "unclear|the posting doesn't say|",
		"Contract":          "|contractor|as a contractor",
	}
	for name, answer := range want {
		if answers[name] != answer {
			t.Errorf("%s = %q, want %q", name, answers[name], answer)
		}
	}
}

func TestADecisionIsRecordedAndActedOn(t *testing.T) {
	service := startAPI(t)
	job, _, _ := service.hub.AddManualJob(context.Background(), store.Actor{Kind: store.ActorOwner}, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/1"})

	status, body := send(t, http.MethodPost, service.url+"/v1/jobs/"+job.ID.String()+"/decision", ownerToken, `{"decision":"pursue"}`)
	var decision store.JobDecision
	if err := json.Unmarshal(body, &decision); status != http.StatusOK || err != nil || decision.Decision != "pursue" {
		t.Fatalf("pursue: %d %s", status, body)
	}
	if _, details := readJobDetails(t, service, job.ID.String()); details.Application == nil {
		t.Error("the pursued job isn't on the pipeline")
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/jobs/"+job.ID.String()+"/decision", ownerToken, `{"decision":"maybe"}`); status != http.StatusBadRequest {
		t.Errorf("an unknown decision: %d, want 400", status)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/jobs/"+uuid.NewString()+"/decision", ownerToken, `{"decision":"later"}`); status != http.StatusNotFound {
		t.Errorf("an unknown job: %d, want 404", status)
	}
}

func TestTheDecisionSignalsCountTheWeeksDecisions(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	for index, decision := range []string{"pursue", "skip", "later"} {
		job, _, _ := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Senior Front-End Engineer " + decision, URL: fmt.Sprintf("https://acme.com/%d", index)})
		if _, err := service.hub.DecideJob(ctx, owner, job.ID, decision, ""); err != nil {
			t.Fatal(err)
		}
	}
	service.hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Senior Front-End Engineer, undecided", URL: "https://acme.com/open"})

	status, body := send(t, http.MethodGet, service.url+"/v1/decision-signals", ownerToken, "")
	var signals struct {
		Decisions            map[string]int `json:"decisions"`
		MedianHoursToDecide  *float64       `json:"median_hours_to_decide"`
		GoodOrUnclearSeen    int            `json:"good_or_unclear_seen"`
		GoodOrUnclearDecided int            `json:"good_or_unclear_decided"`
	}
	if err := json.Unmarshal(body, &signals); status != http.StatusOK || err != nil {
		t.Fatalf("signals: %d %s", status, body)
	}
	if signals.Decisions["pursue"] != 1 || signals.Decisions["skip"] != 1 || signals.Decisions["later"] != 1 || signals.MedianHoursToDecide == nil {
		t.Errorf("signals = %+v", signals)
	}
	if signals.GoodOrUnclearSeen != 4 || signals.GoodOrUnclearDecided != 3 {
		t.Errorf("seen %d, decided %d; want 4 and 3", signals.GoodOrUnclearSeen, signals.GoodOrUnclearDecided)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/decision-queue", ownerToken, ""); status != http.StatusOK {
		t.Errorf("queue: %d", status)
	}
}

func TestTheBaseCVIsSavedAndRenderedAsHTML(t *testing.T) {
	service := startAPI(t)
	if status, _ := send(t, http.MethodGet, service.url+"/v1/cvs/base", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("before a base CV: %d, want 404", status)
	}
	body := `{"basics":{"name":"Ada Lovelace","label":"Senior Engineer"},"work":[{"name":"Acme","position":"Engineer","startDate":"2020-03","highlights":["Shipped it."]}]}`
	status, answer := send(t, http.MethodPut, service.url+"/v1/cvs/base", ownerToken, body)
	var saved store.CV
	if err := json.Unmarshal(answer, &saved); status != http.StatusOK || err != nil || saved.Kind != "base" || saved.Content.Work[0].Highlights[0] != "Shipped it." {
		t.Fatalf("save: %d %s", status, answer)
	}
	status, page := send(t, http.MethodGet, service.url+"/v1/cvs/base/html", ownerToken, "")
	if status != http.StatusOK || !strings.Contains(string(page), "<h1>Ada Lovelace</h1>") || !strings.Contains(string(page), "Mar 2020 - Present") {
		t.Fatalf("html: %d %s", status, page)
	}
	if status, _ := send(t, http.MethodPut, service.url+"/v1/cvs/base", ownerToken, `{"basics":{}}`); status != http.StatusBadRequest {
		t.Errorf("a CV without a name: %d, want 400", status)
	}
}
