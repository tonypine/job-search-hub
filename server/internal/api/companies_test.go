package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheCompaniesListShowsWatchStatusBoardsAndPeople(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	acme, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	zeta, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Zeta", Domain: "zeta.com"})
	if _, _, err := service.hub.AddToWatchList(ctx, owner, acme.ID); err != nil {
		t.Fatal(err)
	}
	openPostings := 3
	if _, err := service.hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: acme.ID, Provider: "lever", BoardToken: "acme", Verified: true, OpenPostingCount: &openPostings}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Ada", "Grace"} {
		if _, _, err := service.hub.AddPerson(ctx, owner, store.PersonInput{CompanyID: acme.ID, Name: name, Relevance: "other", SourceURL: "https://acme.com/team"}); err != nil {
			t.Fatal(err)
		}
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/companies", ownerToken, "")
	var listed struct {
		Companies []store.CompanySummary `json:"companies"`
	}
	if err := json.Unmarshal(body, &listed); status != http.StatusOK || err != nil || len(listed.Companies) != 2 {
		t.Fatalf("list: %d %s", status, body)
	}
	first, second := listed.Companies[0], listed.Companies[1]
	if first.Company.ID != acme.ID || first.WatchedSince == nil || len(first.JobBoards) != 1 || first.PeopleCount != 2 {
		t.Fatalf("acme row = %+v", first)
	}
	if second.Company.ID != zeta.ID || second.WatchedSince != nil || len(second.JobBoards) != 0 || second.PeopleCount != 0 {
		t.Fatalf("zeta row = %+v", second)
	}
}

func TestACompanysDossierIsServedByID(t *testing.T) {
	service := startAPI(t)
	company, _, _ := service.hub.CreateCompany(context.Background(), store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})

	status, body := send(t, http.MethodGet, service.url+"/v1/companies/"+company.ID.String(), ownerToken, "")
	var dossier store.CompanyDossier
	if err := json.Unmarshal(body, &dossier); status != http.StatusOK || err != nil || dossier.Company.ID != company.ID {
		t.Fatalf("dossier: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/companies/7c9e6679-7425-40de-944b-e07fc1f90ae7", ownerToken, ""); status != http.StatusNotFound {
		t.Fatalf("unknown company: %d, want 404", status)
	}
}

func TestTheAppRoutesAreForTheOwnerOnly(t *testing.T) {
	service := startAPI(t)
	agentToken := startTriage(t, service).Token

	for _, attempt := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/companies", ""},
		{http.MethodGet, "/v1/companies/7c9e6679-7425-40de-944b-e07fc1f90ae7", ""},
		{http.MethodGet, "/v1/profile", ""},
		{http.MethodPut, "/v1/profile", `{"body":"changed"}`},
		{http.MethodGet, "/v1/jobs/7c9e6679-7425-40de-944b-e07fc1f90ae7", ""},
		{http.MethodGet, "/v1/job-criteria", ""},
		{http.MethodPut, "/v1/job-criteria", `{}`},
		{http.MethodGet, "/v1/claude-sessions", ""},
		{http.MethodPost, "/v1/claude-sessions", `{}`},
		{http.MethodPost, "/v1/claude-sessions/7c9e6679-7425-40de-944b-e07fc1f90ae7/start", ""},
		{http.MethodGet, "/v1/google", ""},
		{http.MethodPost, "/v1/google/sign-in", ""},
		{http.MethodGet, "/v1/google/check", ""},
	} {
		if status, _ := send(t, attempt.method, service.url+attempt.path, agentToken, attempt.body); status != http.StatusForbidden {
			t.Errorf("agent token on %s %s: %d, want 403", attempt.method, attempt.path, status)
		}
		if status, _ := send(t, attempt.method, service.url+attempt.path, "", attempt.body); status != http.StatusUnauthorized {
			t.Errorf("no token on %s %s: %d, want 401", attempt.method, attempt.path, status)
		}
	}
}
