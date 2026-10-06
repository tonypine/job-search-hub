package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type suggestionsList struct {
	Suggestions []struct {
		Organization string `json:"organization"`
		Source       string `json:"source"`
		Website      string `json:"website"`
		CareersURL   string `json:"careers_url"`
		FittingJobs  int    `json:"fitting_jobs"`
	} `json:"suggestions"`
}

func TestGalleryCompaniesAreSuggestedWhenTheirBoardHasAFittingJob(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	system := store.Actor{Kind: store.ActorSystem}
	if _, err := service.hub.SaveJobCriteria(ctx, owner, store.JobCriteria{
		Technologies: []string{"Go"}, SeniorityLevels: []string{"Senior"}, EligibleLocationTerms: []string{"Americas"},
	}); err != nil {
		t.Fatal(err)
	}
	service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Initech", Domain: "initech.example"})
	if _, err := service.hub.SaveListedGalleryCompanies(ctx, []store.GalleryCompany{
		{Slug: "hooli", Name: "Hooli"}, {Slug: "globex", Name: "Globex"}, {Slug: "initech", Name: "Initech"},
	}); err != nil {
		t.Fatal(err)
	}
	factsPrompt, _ := service.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	// Each company's board holds one job: Hooli's fits, Globex's doesn't, and
	// Initech is already in the hub.
	for slug, posting := range map[string]store.JobPosting{
		"hooli":   {ExternalID: "1", Title: "Senior Go Engineer", Location: "Americas", URL: "https://jobs.ashbyhq.com/hooli/1"},
		"globex":  {ExternalID: "2", Title: "Accountant", Location: "Berlin", URL: "https://jobs.ashbyhq.com/globex/2"},
		"initech": {ExternalID: "3", Title: "Senior Go Engineer", Location: "Americas", URL: "https://jobs.ashbyhq.com/initech/3"},
	} {
		if err := service.hub.RecordGalleryPage(ctx, slug, store.GalleryPage{
			Website: "https://" + slug + ".example/", CareersURL: "https://jobs.ashbyhq.com/" + slug, Provider: "ashby", BoardToken: slug,
		}); err != nil {
			t.Fatal(err)
		}
		board, err := service.hub.SaveFoundJobBoard(ctx, system, store.FoundJobBoardInput{
			CompanyName: map[string]string{"hooli": "Hooli", "globex": "Globex", "initech": "Initech"}[slug], Provider: "ashby", BoardToken: slug,
			BoardURL: "https://jobs.ashbyhq.com/" + slug, OpenPostingCount: 1, FoundBy: store.JobBoardFoundByDiscovery,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.hub.RecordGalleryBoardCheck(ctx, slug, &board.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.hub.SyncBoardJobs(ctx, system, board, []store.JobPosting{posting}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	jobs, _, _ := service.hub.ListJobs(ctx, store.JobFilter{Status: store.JobStatusOpen})
	for _, item := range jobs {
		if err := service.hub.SaveJobFacts(ctx, store.NewJobFacts{
			JobID: item.Job.ID, PromptID: factsPrompt.ID, Model: "test-model", TextHash: []byte{1}, Facts: json.RawMessage(`{"years_of_experience":6}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/company-suggestions", ownerToken, "")
	var list suggestionsList
	if err := json.Unmarshal(body, &list); status != http.StatusOK || err != nil || len(list.Suggestions) != 1 {
		t.Fatalf("suggestions: %d %s; want Hooli alone", status, body)
	}
	if hooli := list.Suggestions[0]; hooli.Organization != "Hooli" || hooli.Source != "startups_gallery" || hooli.FittingJobs != 1 ||
		hooli.Website != "https://hooli.example/" || hooli.CareersURL != "https://jobs.ashbyhq.com/hooli" {
		t.Errorf("suggestion = %+v; want Hooli from startups.gallery, with its fitting job, site and careers link", hooli)
	}

	follows := "Organization,Followed On\nHooli,Wed Jun 24 12:43:42 UTC 2026\n"
	send(t, http.MethodPost, service.url+"/v1/linkedin/company-follows/import", ownerToken, follows)
	status, body = send(t, http.MethodGet, service.url+"/v1/company-suggestions", ownerToken, "")
	list = suggestionsList{}
	if err := json.Unmarshal(body, &list); status != http.StatusOK || err != nil || len(list.Suggestions) != 1 || list.Suggestions[0].Source != "linkedin" {
		t.Errorf("suggestions: %d %s; want Hooli once, as followed on LinkedIn", status, body)
	}
}
