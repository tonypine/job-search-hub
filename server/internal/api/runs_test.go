package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

var runOwner = store.Actor{Kind: store.ActorOwner}

func listRuns(t *testing.T, service apiUnderTest, query url.Values) store.RunPage {
	t.Helper()
	status, answer := send(t, http.MethodGet, service.url+"/v1/runs?"+query.Encode(), ownerToken, "")
	var page store.RunPage
	if err := json.Unmarshal(answer, &page); err != nil || status != http.StatusOK {
		t.Fatalf("runs %s: %d %s", query.Encode(), status, answer)
	}
	return page
}

func TestAJobsBriefRunNamesTheJobAndItsCompany(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	company, _, _ := service.hub.CreateCompany(ctx, runOwner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	job, _, _ := service.hub.AddManualJob(ctx, runOwner, store.ManualJobInput{CompanyID: &company.ID, Title: "Frontend Engineer", URL: "https://acme.com/jobs/1"})
	other, _, _ := service.hub.AddManualJob(ctx, runOwner, store.ManualJobInput{Title: "Backend Engineer", URL: "https://acme.com/jobs/2"})
	now := time.Now()
	for _, run := range []store.NewTaskRun{
		{Kind: store.AgentPromptKindJobBrief, SubjectID: &other.ID, StartedAt: now.Add(-time.Hour)},
		{Kind: store.AgentPromptKindJobFacts, SubjectID: &job.ID, StartedAt: now.Add(-time.Minute)},
		{Kind: store.AgentPromptKindJobBrief, SubjectID: &job.ID, StartedAt: now.Add(-2 * time.Minute), StartedByOwner: true},
	} {
		run.Provider, run.Model, run.Outcome, run.Duration = "Local model", "qwen", "succeeded", time.Second
		if _, err := service.hub.RecordTaskRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}

	page := listRuns(t, service, url.Values{"kind": {"job_brief"}})
	if len(page.Runs) != 2 || page.NextCursor != "" {
		t.Fatalf("job brief runs = %+v", page)
	}
	newest := page.Runs[0]
	if subject := newest.Subject; subject == nil || subject.Type != "job" || *subject.ID != job.ID || subject.Title != "Frontend Engineer" ||
		subject.CompanyID == nil || *subject.CompanyID != company.ID || subject.CompanyName != "Acme" {
		t.Errorf("the newest brief's subject = %+v", subject)
	}
	if newest.Model != "qwen" || newest.Provider != "Local model" || !newest.StartedByOwner {
		t.Errorf("the newest brief = %+v", newest)
	}
	if subject := page.Runs[1].Subject; subject == nil || subject.CompanyID != nil || subject.CompanyName != "" {
		t.Errorf("a job with no company names one: %+v", subject)
	}

	first := listRuns(t, service, url.Values{"limit": {"2"}})
	if len(first.Runs) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second := listRuns(t, service, url.Values{"limit": {"2"}, "cursor": {first.NextCursor}})
	if len(second.Runs) != 1 || second.NextCursor != "" || second.Runs[0].Subject.ID == nil || *second.Runs[0].Subject.ID != other.ID {
		t.Errorf("second page = %+v", second)
	}
}

func TestACompanysResearchIsListedForTheCompany(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	company, _, _ := service.hub.CreateCompany(ctx, runOwner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	research := func(input string) store.AgentRun {
		t.Helper()
		status, answer := send(t, http.MethodPost, service.url+"/v1/agent-runs", ownerToken, `{"kind":"company_triage","input":"`+input+`"}`)
		var started struct {
			AgentRun store.AgentRun `json:"agent_run"`
		}
		if err := json.Unmarshal(answer, &started); err != nil || status != http.StatusCreated {
			t.Fatalf("start: %d %s", status, answer)
		}
		return started.AgentRun
	}
	finish := func(run store.AgentRun) {
		t.Helper()
		if status, answer := send(t, http.MethodPost, service.url+"/v1/agent-runs/"+run.ID.String()+"/finish", ownerToken,
			`{"status":"succeeded","cost_usd_estimate":0.31}`); status != http.StatusOK {
			t.Fatalf("finish: %d %s", status, answer)
		}
	}

	known := research("https://www.acme.com/about")
	finish(known)
	page := listRuns(t, service, url.Values{"subject_id": {company.ID.String()}})
	if len(page.Runs) != 1 || page.Runs[0].ID != known.ID {
		t.Fatalf("runs for the company = %+v", page)
	}
	if run := page.Runs[0]; run.Kind != "company_triage" || run.Subject.Type != "company" || run.Subject.Title != "Acme" ||
		run.Outcome != "succeeded" || *run.CostUSDEstimate != "0.31" || !run.StartedByOwner {
		t.Errorf("the research run = %+v", run)
	}

	// A company the hub didn't know is linked once the run adds it.
	unknown := research("newco.example")
	added, _, err := service.hub.CreateCompany(ctx, store.Actor{Kind: store.ActorAgentRun, AgentRunID: unknown.ID},
		store.NewCompany{Name: "Newco", Domain: "newco.example"})
	if err != nil {
		t.Fatal(err)
	}
	if page := listRuns(t, service, url.Values{"subject_id": {added.ID.String()}}); len(page.Runs) != 0 {
		t.Fatalf("a running run is linked before it ends: %+v", page)
	}
	finish(unknown)
	page = listRuns(t, service, url.Values{"subject_type": {"company"}, "subject_id": {added.ID.String()}})
	if len(page.Runs) != 1 || page.Runs[0].ID != unknown.ID || page.Runs[0].Subject.Title != "Newco" {
		t.Errorf("runs for the added company = %+v", page)
	}
}

func TestRunsRefuseAFilterTheyCantRead(t *testing.T) {
	service := startAPI(t)
	for _, query := range []string{"kind=job_dream", "outcome=maybe", "subject_id=acme", "since=yesterday", "until=2026-13-01", "limit=-1", "cursor=nope"} {
		if status, answer := send(t, http.MethodGet, service.url+"/v1/runs?"+query, ownerToken, ""); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", query, status, answer)
		}
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/runs", "", ""); status != http.StatusUnauthorized {
		t.Errorf("without a token: %d, want 401", status)
	}
}
