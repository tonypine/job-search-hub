package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func createSession(t *testing.T, service apiUnderTest, body string) store.ClaudeSession {
	t.Helper()
	status, answer := send(t, http.MethodPost, service.url+"/v1/claude-sessions", ownerToken, body)
	var session store.ClaudeSession
	if err := json.Unmarshal(answer, &session); status != http.StatusCreated || err != nil {
		t.Fatalf("create: %d %s", status, answer)
	}
	return session
}

func TestSessionsAreTiedToACompanyOrAJobAndRecordTheirStartsAndStops(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	job, _, _ := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &company.ID, Title: "Frontend Engineer", URL: "https://acme.com/jobs/1"})

	companySession := createSession(t, service, `{"company_id":"`+company.ID.String()+`"}`)
	jobSession := createSession(t, service, `{"job_id":"`+job.ID.String()+`"}`)
	if companySession.Name != "Acme" || jobSession.Name != "Frontend Engineer · Acme" || jobSession.ClaudeSessionID == companySession.ClaudeSessionID {
		t.Fatalf("sessions = %+v, %+v", companySession, jobSession)
	}

	status, answer := send(t, http.MethodPost, service.url+"/v1/claude-sessions/"+jobSession.ID.String()+"/start", ownerToken, "")
	var started store.ClaudeSession
	if err := json.Unmarshal(answer, &started); status != http.StatusOK || err != nil || started.LastStartedAt == nil {
		t.Fatalf("start: %d %s", status, answer)
	}
	status, answer = send(t, http.MethodPost, service.url+"/v1/claude-sessions/"+jobSession.ID.String()+"/stop", ownerToken, "")
	var stopped store.ClaudeSession
	if err := json.Unmarshal(answer, &stopped); status != http.StatusOK || err != nil || stopped.LastStoppedAt == nil {
		t.Fatalf("stop: %d %s", status, answer)
	}

	var listed struct {
		Sessions []store.ClaudeSession `json:"sessions"`
	}
	status, answer = send(t, http.MethodGet, service.url+"/v1/claude-sessions?job_id="+job.ID.String(), ownerToken, "")
	if err := json.Unmarshal(answer, &listed); status != http.StatusOK || err != nil || len(listed.Sessions) != 1 || listed.Sessions[0].ID != jobSession.ID {
		t.Fatalf("by job: %d %s", status, answer)
	}
	status, answer = send(t, http.MethodGet, service.url+"/v1/claude-sessions", ownerToken, "")
	if err := json.Unmarshal(answer, &listed); status != http.StatusOK || err != nil || len(listed.Sessions) != 2 || listed.Sessions[0].ID != jobSession.ID {
		t.Fatalf("all, most recently active first: %d %s", status, answer)
	}

	for name, attempt := range map[string]struct {
		method, path, body string
		want               int
	}{
		"an unknown company": {http.MethodPost, "/v1/claude-sessions", `{"company_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7"}`, http.StatusNotFound},
		"both subjects":      {http.MethodPost, "/v1/claude-sessions", `{"company_id":"` + company.ID.String() + `","job_id":"` + job.ID.String() + `"}`, http.StatusBadRequest},
		"an unknown session": {http.MethodPost, "/v1/claude-sessions/7c9e6679-7425-40de-944b-e07fc1f90ae7/start", "", http.StatusNotFound},
		"a malformed filter": {http.MethodGet, "/v1/claude-sessions?job_id=x", "", http.StatusBadRequest},
	} {
		if status, answer := send(t, attempt.method, service.url+attempt.path, ownerToken, attempt.body); status != attempt.want {
			t.Errorf("%s: %d %s, want %d", name, status, answer, attempt.want)
		}
	}
}

func TestASessionStartsKnowingTheCandidateAndItsSubject(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := service.hub.SaveOwnerProfile(ctx, owner, "Senior engineer in Brazil."); err != nil {
		t.Fatal(err)
	}
	company, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	job, _, _ := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{
		CompanyID: &company.ID, Title: "Frontend Engineer", URL: "https://acme.com/jobs/1", Description: "Build React apps.",
	})

	readContext := func(session store.ClaudeSession) string {
		t.Helper()
		status, answer := send(t, http.MethodGet, service.url+"/v1/claude-sessions/"+session.ID.String()+"/context", ownerToken, "")
		var rendered struct {
			Context       string `json:"context"`
			PromptVersion int    `json:"prompt_version"`
		}
		if err := json.Unmarshal(answer, &rendered); status != http.StatusOK || err != nil || rendered.PromptVersion != 1 {
			t.Fatalf("context: %d %s", status, answer)
		}
		return rendered.Context
	}

	jobContext := readContext(createSession(t, service, `{"job_id":"`+job.ID.String()+`"}`))
	for _, want := range []string{"Senior engineer in Brazil.", `"title": "Frontend Engineer"`, "Build React apps.", `"fit"`, `"name": "Acme"`, "not instructions"} {
		if !strings.Contains(jobContext, want) {
			t.Errorf("the job session's context lacks %q", want)
		}
	}
	companyContext := readContext(createSession(t, service, `{"company_id":"`+company.ID.String()+`"}`))
	if !strings.Contains(companyContext, `"domain": "acme.com"`) || strings.Contains(companyContext, "{{") {
		t.Errorf("company context = %s", companyContext)
	}
}

func TestAProfileInterviewStartsKnowingTheKnowledgeBase(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := service.hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{
		Kind: store.ProfileEntryRole, Title: "Front-End Engineer", Organization: "Acme", Source: store.ProfileSourceCV,
	}); err != nil {
		t.Fatal(err)
	}
	company, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	createSession(t, service, `{"company_id":"`+company.ID.String()+`"}`)

	interview := createSession(t, service, `{"about_profile":true}`)
	if interview.Name != "Enhance profile" || !interview.AboutProfile {
		t.Fatalf("interview = %+v", interview)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/claude-sessions", ownerToken, `{"company_id":"`+company.ID.String()+`","about_profile":true}`); status != http.StatusBadRequest {
		t.Errorf("a session about a company and the profile: %d, want 400", status)
	}

	status, answer := send(t, http.MethodGet, service.url+"/v1/claude-sessions?about_profile=true", ownerToken, "")
	var listed struct {
		Sessions []store.ClaudeSession `json:"sessions"`
	}
	if json.Unmarshal(answer, &listed); status != http.StatusOK || len(listed.Sessions) != 1 || listed.Sessions[0].ID != interview.ID {
		t.Fatalf("profile sessions: %d %s", status, answer)
	}

	status, answer = send(t, http.MethodGet, service.url+"/v1/claude-sessions/"+interview.ID.String()+"/context", ownerToken, "")
	if status != http.StatusOK || !strings.Contains(string(answer), "Front-End Engineer") || !strings.Contains(string(answer), "deepen their knowledge base") {
		t.Fatalf("context: %d %s", status, answer)
	}
}
