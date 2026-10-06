package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

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

func TestTheCompaniesListCountsTheOpenJobsThatPassTheScreenAndTheirBestMatch(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := service.hub.SaveJobCriteria(ctx, owner, store.JobCriteria{
		Technologies: []string{"Go"}, SeniorityLevels: []string{"Senior"}, EligibleLocationTerms: []string{"Americas"},
	}); err != nil {
		t.Fatal(err)
	}
	acme, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	zeta, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Zeta", Domain: "zeta.example"})
	prompt, _ := service.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	hash, _ := service.hub.GetKnowledgeHash(ctx)
	// Two of Acme's jobs pass the screen, briefed possible and strong; the
	// third fails it, so its strong brief doesn't count.
	for index, job := range []struct{ title, location, match string }{
		{"Senior Go Engineer", "Americas", "possible"},
		{"Senior Go Engineer, Platform", "Americas", "strong"},
		{"Accountant", "Berlin", "strong"},
	} {
		added, _, err := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{
			CompanyID: &acme.ID, Title: job.title, Location: job.location, URL: fmt.Sprintf("https://acme.example/jobs/%d", index),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.hub.SaveJobFacts(ctx, store.NewJobFacts{
			JobID: added.ID, PromptID: prompt.ID, Model: "test-model", TextHash: []byte{1}, Facts: json.RawMessage(`{"years_of_experience":6}`),
		}); err != nil {
			t.Fatal(err)
		}
		if err := service.hub.SaveJobBrief(ctx, store.JobBrief{JobID: added.ID, Tier: store.JobBriefTierPre, PromptID: prompt.ID, Model: "local",
			Match: job.match, Reason: "Go.", KnowledgeHash: hash}); err != nil {
			t.Fatal(err)
		}
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/companies", ownerToken, "")
	var listed struct {
		Companies []struct {
			Company                store.Company `json:"company"`
			FittingJobs            int           `json:"fitting_jobs"`
			BestMatch              *string       `json:"best_match"`
			NewestFittingJobSeenAt *time.Time    `json:"newest_fitting_job_seen_at"`
			KnownPeople            []string      `json:"known_people"`
		} `json:"companies"`
	}
	if err := json.Unmarshal(body, &listed); status != http.StatusOK || err != nil || len(listed.Companies) != 2 {
		t.Fatalf("list: %d %s", status, body)
	}
	acmeRow, zetaRow := listed.Companies[0], listed.Companies[1]
	if acmeRow.Company.ID != acme.ID || acmeRow.FittingJobs != 2 || acmeRow.BestMatch == nil || *acmeRow.BestMatch != "strong" ||
		acmeRow.NewestFittingJobSeenAt == nil || acmeRow.KnownPeople == nil {
		t.Fatalf("Acme = %+v; want 2 jobs that pass, the best strong, seen now", acmeRow)
	}
	if zetaRow.Company.ID != zeta.ID || zetaRow.FittingJobs != 0 || zetaRow.BestMatch != nil || zetaRow.NewestFittingJobSeenAt != nil {
		t.Fatalf("Zeta = %+v; want nothing open", zetaRow)
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
	var threads struct {
		Applications []store.CompanyApplication `json:"applications"`
		Mail         []store.MailMessage        `json:"mail"`
	}
	if err := json.Unmarshal(body, &threads); err != nil || threads.Applications == nil || threads.Mail == nil {
		t.Fatalf("a company with no threads should list empty applications and mail: %s", body)
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
		{http.MethodGet, "/v1/updates", ""},
		{http.MethodPost, "/v1/updates", `{"kind":"x","title":"y"}`},
		{http.MethodPost, "/v1/updates/seen", `{"all":true}`},
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

func TestACompanysPageHoldsItsApplicationsAndLatestMail(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	acme, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	zeta, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Zeta", Domain: "zeta.com"})
	job, _, _ := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &acme.ID, Title: "Engineer", URL: "https://acme.com/jobs/1"})
	if _, _, err := service.hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.hub.AddApplication(ctx, owner, store.ApplicationInput{CompanyID: &zeta.ID}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for index, mail := range []struct {
		subject string
		company store.Company
		sentAt  time.Time
	}{
		{"Your application to Acme", acme, now.Add(-48 * time.Hour)},
		{"Interview with Acme", acme, now.Add(-time.Hour)},
		{"Hello from Zeta", zeta, now},
	} {
		message, _, err := service.hub.RecordMailMessage(ctx, store.NewMailMessage{
			GmailMessageID: fmt.Sprintf("message-%d", index), ThreadID: fmt.Sprintf("thread-%d", index), Direction: store.MailReceived,
			Sender: "jobs@" + mail.company.Domain, Recipients: "owner@example.com", Subject: mail.subject, SentAt: mail.sentAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.hub.SaveMailMatch(ctx, message.ID, store.MailMatch{CompanyID: mail.company.ID, MatchedBy: store.MatchedByDomain}); err != nil {
			t.Fatal(err)
		}
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/companies/"+acme.ID.String(), ownerToken, "")
	var page struct {
		Company      store.Company              `json:"company"`
		Applications []store.CompanyApplication `json:"applications"`
		Mail         []store.MailMessage        `json:"mail"`
	}
	if err := json.Unmarshal(body, &page); status != http.StatusOK || err != nil || page.Company.ID != acme.ID {
		t.Fatalf("company: %d %s", status, body)
	}
	if len(page.Applications) != 1 || page.Applications[0].PhaseName != "Saved" || page.Applications[0].JobTitle == nil || *page.Applications[0].JobTitle != "Engineer" {
		t.Fatalf("applications = %+v; want Acme's one card, in Saved", page.Applications)
	}
	if len(page.Mail) != 2 || page.Mail[0].Subject != "Interview with Acme" || page.Mail[1].Subject != "Your application to Acme" {
		t.Fatalf("mail = %+v; want Acme's two messages, newest first", page.Mail)
	}
}

func TestACompanysLatestMailLeavesOutNoiseAndJobAlerts(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	acme, _, _ := service.hub.CreateCompany(ctx, store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	now := time.Now()
	for index, mail := range []struct {
		subject, class string
		sentAt         time.Time
	}{
		{"Interview with Acme", store.MailInterviewInvite, now.Add(-3 * time.Hour)},
		{"The Acme newsletter", store.MailNoise, now.Add(-2 * time.Hour)},
		{"New jobs at Acme", store.MailJobAlert, now.Add(-time.Hour)},
		{"Not read yet", "", now},
	} {
		message, _, err := service.hub.RecordMailMessage(ctx, store.NewMailMessage{
			GmailMessageID: fmt.Sprintf("message-%d", index), ThreadID: fmt.Sprintf("thread-%d", index), Direction: store.MailReceived,
			Sender: "news@acme.com", Recipients: "owner@example.com", Subject: mail.subject, SentAt: mail.sentAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.hub.SaveMailMatch(ctx, message.ID, store.MailMatch{CompanyID: acme.ID, MatchedBy: store.MatchedByDomain}); err != nil {
			t.Fatal(err)
		}
		if mail.class == "" {
			continue
		}
		if err := service.hub.SaveMailClassification(ctx, message.ID, store.MailClassification{Class: mail.class, ClassifiedBy: store.ClassifiedByRule, Reason: "test"}); err != nil {
			t.Fatal(err)
		}
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/companies/"+acme.ID.String(), ownerToken, "")
	var page struct {
		Mail            []store.MailMessage `json:"mail"`
		FoldedMailCount int                 `json:"folded_mail_count"`
	}
	if err := json.Unmarshal(body, &page); status != http.StatusOK || err != nil {
		t.Fatalf("company: %d %s", status, body)
	}
	if len(page.Mail) != 2 || page.Mail[0].Subject != "Not read yet" || page.Mail[1].Subject != "Interview with Acme" {
		t.Fatalf("mail = %+v; want the unread message and the invite, without the newsletter and the alert", page.Mail)
	}
	if page.FoldedMailCount != 2 {
		t.Fatalf("folded_mail_count = %d, want 2", page.FoldedMailCount)
	}
}
