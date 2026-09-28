package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const madeUpConnections = "Notes:\n\"About missing emails.\"\n\nFirst Name,Last Name,URL,Email Address,Company,Position,Connected On\n" +
	"Ada,Lovelace,https://www.linkedin.com/in/ada-example,,\"Acme, Inc.\",Engineering Manager,28 Sep 2026\n" +
	"Grace,Hopper,https://www.linkedin.com/in/grace-example,,Globex,Engineer,03 Jan 2019\n"

func TestConnectionsAreImportedOnceAndTiedToKnownCompanies(t *testing.T) {
	service := startAPI(t)
	acme, _, err := service.hub.CreateCompany(context.Background(), store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodPost, service.url+"/v1/connections/import", ownerToken, madeUpConnections)
	var first struct {
		Added, Updated, Matched, Skipped int
	}
	if json.Unmarshal(body, &first); status != http.StatusOK || first.Added != 2 || first.Matched != 1 {
		t.Fatalf("first import: %d %s", status, body)
	}
	status, body = send(t, http.MethodPost, service.url+"/v1/connections/import", ownerToken, madeUpConnections)
	var second struct{ Added, Updated int }
	if json.Unmarshal(body, &second); status != http.StatusOK || second.Added != 0 || second.Updated != 2 {
		t.Fatalf("second import: %d %s; want the same two updated", status, body)
	}

	atAcme, _ := service.hub.ListCompanyConnections(context.Background(), acme.ID)
	if len(atAcme) != 1 || atAcme[0].FirstName != "Ada" {
		t.Fatalf("connections at Acme = %+v", atAcme)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/connections", ownerToken, "")
	var summary store.ConnectionsSummary
	if json.Unmarshal(body, &summary); status != http.StatusOK || summary.Count != 2 || summary.Matched != 1 || summary.LastImportedAt == nil {
		t.Fatalf("summary: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/connections/import", ownerToken, "Name,Email\n"); status != http.StatusBadRequest {
		t.Fatalf("another file: %d", status)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/connections", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("without the owner token: %d", status)
	}
}

func TestMessagesGiveConnectionsTheirHistoryAndReimportReplacesThem(t *testing.T) {
	service := startAPI(t)
	connections := "First Name,Last Name,URL,Email Address,Company,Position,Connected On\n" +
		"Rita,Recruiter,https://www.linkedin.com/in/rita-example/,,Acme,Recruiter,01 Jan 2020\n" +
		"Ada,Lovelace,https://www.linkedin.com/in/ada-example,,Globex,Engineer,01 Jan 2020\n"
	if status, body := send(t, http.MethodPost, service.url+"/v1/connections/import", ownerToken, connections); status != http.StatusOK {
		t.Fatalf("connections: %d %s", status, body)
	}
	messages := `"CONVERSATION ID","CONVERSATION TITLE","FROM","SENDER PROFILE URL","TO","RECIPIENT PROFILE URLS","DATE","SUBJECT","CONTENT","FOLDER","ATTACHMENTS"
"c1","","Rita","https://www.linkedin.com/in/rita-example","Owner","https://www.linkedin.com/in/owner-example","2025-03-01 10:00:00 UTC","","A role","INBOX",""
"c1","","Owner","https://www.linkedin.com/in/owner-example","Rita","https://www.linkedin.com/in/rita-example","2025-03-02 10:00:00 UTC","","Thanks","INBOX",""
"c2","","Owner","https://www.linkedin.com/in/owner-example","Rita","https://www.linkedin.com/in/rita-example","2026-01-05 10:00:00 UTC","","Checking in","INBOX",""
`
	for attempt := 1; attempt <= 2; attempt++ {
		status, body := send(t, http.MethodPost, service.url+"/v1/linkedin/messages/import", ownerToken, messages)
		var imported store.MessagesImport
		if json.Unmarshal(body, &imported); status != http.StatusOK || imported.Conversations != 2 || imported.Messages != 3 || imported.ConnectionsWithHistory != 1 {
			t.Fatalf("import %d: %d %s", attempt, status, body)
		}
	}
	var messageRows int
	service.pool.QueryRow(context.Background(), `SELECT count(*) FROM linkedin_messages`).Scan(&messageRows)
	if messageRows != 3 {
		t.Fatalf("stored messages = %d after two imports; want 3", messageRows)
	}
	var conversations, messageCount int
	var theyWroteFirst bool
	service.pool.QueryRow(context.Background(), `SELECT conversation_count, message_count, they_wrote_first FROM connections WHERE first_name = 'Rita'`).
		Scan(&conversations, &messageCount, &theyWroteFirst)
	if conversations != 2 || messageCount != 3 || !theyWroteFirst {
		t.Fatalf("Rita's history = %d conversations, %d messages, wrote first %v", conversations, messageCount, theyWroteFirst)
	}
	var startedByOwner bool
	service.pool.QueryRow(context.Background(), `SELECT started_by_owner FROM linkedin_conversations WHERE linkedin_id = 'c2'`).Scan(&startedByOwner)
	if !startedByOwner {
		t.Fatal("c2 was started by the owner")
	}
}

func TestTheRecruitersListCountsTheOpeningsAtTheirCompany(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	at := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	if _, err := service.hub.ImportLinkedInMessages(ctx, owner, []store.NewLinkedInMessage{{
		ConversationID: "c1", SenderName: "Rita Recruiter", SenderProfileURL: "https://www.linkedin.com/in/rita-example",
		RecipientProfileURLs: []string{"https://www.linkedin.com/in/owner-example"}, SentAt: at, Content: "A role at Globex",
	}}); err != nil {
		t.Fatal(err)
	}
	awaiting, _ := service.hub.ListConversationsAwaitingClassification(ctx, 10)
	if err := service.hub.SaveConversationClassification(ctx, awaiting[0].ID, store.ConversationClassification{
		Class: "recruiter_outreach", ClassifiedBy: store.ClassifiedByModel, HiringCompany: "Globex Inc.", Role: "Engineer",
	}); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(48 * time.Hour)
	if _, err := service.hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, []store.JobPosting{{
		ExternalID: "1", CompanyName: "Globex", Title: "Frontend Engineer", Location: "Worldwide",
		URL: "https://himalayas.app/companies/globex/jobs/1", ExpiresAt: &expiresAt,
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/recruiters", ownerToken, "")
	var list struct {
		Recruiters []struct {
			StartedByName string `json:"started_by_name"`
			HiringCompany string `json:"hiring_company"`
			OpenJobs      int    `json:"open_jobs"`
		} `json:"recruiters"`
	}
	if json.Unmarshal(body, &list); status != http.StatusOK || len(list.Recruiters) != 1 || list.Recruiters[0].OpenJobs != 1 ||
		list.Recruiters[0].StartedByName != "Rita Recruiter" {
		t.Fatalf("recruiters: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/linkedin/conversations/"+awaiting[0].ID.String()+"/messages", ownerToken, "")
	if status != http.StatusOK || !strings.Contains(string(body), "A role at Globex") {
		t.Fatalf("messages: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/recruiters/"+awaiting[0].ID.String()+"/reply-prompt", ownerToken, "")
	if status != http.StatusOK || !strings.Contains(string(body), "A role at Globex") || !strings.Contains(string(body), "Open roles at their company") {
		t.Fatalf("reply prompt: %d %s; want the conversation and the openings section in it", status, body)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/recruiters/"+uuid.NewString()+"/reply-prompt", ownerToken, ""); status != http.StatusNotFound {
		t.Fatalf("an unknown conversation: %d", status)
	}
}

func TestLinkedInApplicationsJoinThePipelineAsActiveCardsOrHistory(t *testing.T) {
	service := startAPI(t)
	recent := time.Now().AddDate(0, 0, -10).Format("1/2/06, 3:04 PM")
	applications := "Application Date,Contact Email,Contact Phone Number,Company Name,Job Title,Job Url,Resume Name,Question And Answers\n" +
		"\"" + recent + "\",,,Acme,Frontend Engineer,http://www.linkedin.com/jobs/view/1,resume.pdf,\n" +
		"\"3/24/23, 4:11 PM\",,,Globex,Backend Engineer,http://www.linkedin.com/jobs/view/2,resume.pdf,\n"
	saved := "Saved Date,Job Url,Job Title,Company Name\n" +
		"\"" + recent + "\",http://www.linkedin.com/jobs/view/3,Staff Engineer,Initech\n" +
		"\"6/1/20, 11:51 AM\",http://www.linkedin.com/jobs/view/4,Old Save,Umbrella\n"

	for attempt := 1; attempt <= 2; attempt++ {
		status, body := send(t, http.MethodPost, service.url+"/v1/linkedin/applications/import", ownerToken, applications)
		var imported store.LinkedInJobsImport
		json.Unmarshal(body, &imported)
		want := store.LinkedInJobsImport{Active: 1, Closed: 1}
		if attempt == 2 {
			want = store.LinkedInJobsImport{AlreadyOnBoard: 2}
		}
		if status != http.StatusOK || imported != want {
			t.Fatalf("applications import %d: %d %s", attempt, status, body)
		}
	}
	status, body := send(t, http.MethodPost, service.url+"/v1/linkedin/saved-jobs/import", ownerToken, saved)
	var imported store.LinkedInJobsImport
	if json.Unmarshal(body, &imported); status != http.StatusOK || imported != (store.LinkedInJobsImport{Active: 1, Skipped: 1}) {
		t.Fatalf("saved import: %d %s", status, body)
	}

	cards, _ := service.hub.ListPipelineCards(context.Background())
	phases := map[string]string{}
	for _, card := range cards {
		phases[*card.JobTitle] = card.Application.ClosedReason
	}
	if len(cards) != 3 || phases["Backend Engineer"] != "Applied on LinkedIn on 2023-03-24; no outcome recorded" {
		t.Fatalf("cards = %+v", phases)
	}
	openJobs, _, _ := service.hub.ListJobs(context.Background(), store.JobFilter{Status: store.JobStatusOpen})
	for _, job := range openJobs {
		if job.Job.Title == "Backend Engineer" {
			t.Fatal("the job of an application kept as history should be closed")
		}
	}
}

func TestFollowedCompaniesAreSuggestedByTheirOpenings(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	follows := "Organization,Followed On\nGlobex,Wed Jun 24 12:43:42 UTC 2026\nInitech,Thu Oct 24 16:49:32 UTC 2024\nAcme,Wed Mar 20 18:29:04 UTC 2013\n"
	if status, body := send(t, http.MethodPost, service.url+"/v1/linkedin/company-follows/import", ownerToken, follows); status != http.StatusOK || !strings.Contains(string(body), `"stored":3`) {
		t.Fatalf("import: %d %s", status, body)
	}
	service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	expiresAt := time.Now().Add(48 * time.Hour)
	if _, err := service.hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, []store.JobPosting{{
		ExternalID: "1", CompanyName: "Initech", Title: "Frontend Engineer", Location: "Worldwide",
		URL: "https://himalayas.app/companies/initech/jobs/1", ExpiresAt: &expiresAt,
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodGet, service.url+"/v1/company-suggestions", ownerToken, "")
	var list struct {
		Suggestions []struct {
			Organization string `json:"organization"`
			OpenJobs     int    `json:"open_jobs"`
		} `json:"suggestions"`
	}
	if json.Unmarshal(body, &list); status != http.StatusOK || len(list.Suggestions) != 2 || list.Suggestions[0].Organization != "Initech" ||
		list.Suggestions[0].OpenJobs != 1 {
		t.Fatalf("suggestions: %d %s; want Initech first with its opening, and Acme left out as already in the hub", status, body)
	}
}

func TestTheLinkedInProfileIsImportedComparedAndItsEmployersNotSuggested(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	if _, err := service.hub.SaveJobCriteria(ctx, store.Actor{Kind: store.ActorOwner}, store.JobCriteria{
		Roles: []string{"Senior Software Engineer", "Senior Front-End Engineer"}, EligibleLocationTerms: []string{"Brazil", "LATAM"},
	}); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(map[string]any{"files": map[string]string{
		"Profile.csv":                "First Name,Last Name,Maiden Name,Address,Birth Date,Headline,Summary,Industry,Zip Code,Geo Location,Twitter Handles,Websites,Instant Messengers\nOwner,Example,,Secret Street 1,1 Jan 1990,Senior Software Engineer,Builds products,Software,00000,Brazil,,,\n",
		"Positions.csv":              "Company Name,Title,Description,Location,Started On,Finished On\nGlobex,Senior Engineer,,Remote,Jan 2024,\nUmbrella Health,Engineer,,Remote,Jan 2020,Dec 2023\n",
		"Skills.csv":                 "Name\nReact\nTypeScript\n",
		"Job Seeker Preferences.csv": "Locations,Industries,Company Employee Count,Preferred Job Types,Job Titles,Open To Recruiters\n,Software Development,Over 1 employees,Full-time,Senior Software Engineer,No\n",
	}})
	status, body := send(t, http.MethodPost, service.url+"/v1/linkedin/profile/import", ownerToken, string(request))
	if status != http.StatusOK || !strings.Contains(string(body), "Positions.csv") {
		t.Fatalf("import: %d %s", status, body)
	}

	status, body = send(t, http.MethodGet, service.url+"/v1/linkedin/profile", ownerToken, "")
	var read struct {
		Profile             store.LinkedInProfile `json:"profile"`
		CriteriaDifferences []string              `json:"criteria_differences"`
	}
	json.Unmarshal(body, &read)
	if status != http.StatusOK || read.Profile.Headline != "Senior Software Engineer" || len(read.Profile.Skills) != 2 || strings.Contains(string(body), "Secret Street") {
		t.Fatalf("profile: %d %s; want the headline and skills, and never the address", status, body)
	}
	differences := strings.Join(read.CriteriaDifferences, "\n")
	if !strings.Contains(differences, `"Senior Front-End Engineer"`) || !strings.Contains(differences, "Open to recruiters is off") ||
		!strings.Contains(differences, "no locations") || strings.Contains(differences, `want "Senior Software Engineer"`) {
		t.Fatalf("differences = %q", differences)
	}

	follows := "Organization,Followed On\nGlobex,Wed Jun 24 12:43:42 UTC 2026\nInitech,Thu Oct 24 16:49:32 UTC 2024\nUmbrella,Thu Oct 24 16:49:32 UTC 2024\n"
	send(t, http.MethodPost, service.url+"/v1/linkedin/company-follows/import", ownerToken, follows)
	status, body = send(t, http.MethodGet, service.url+"/v1/company-suggestions", ownerToken, "")
	if status != http.StatusOK || strings.Contains(string(body), "Globex") || strings.Contains(string(body), "Umbrella") || !strings.Contains(string(body), "Initech") {
		t.Fatalf("suggestions: %d %s; a former employer is never suggested", status, body)
	}
}
