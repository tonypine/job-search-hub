package main

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

func TestTheSeedFillsEveryPageQAJudgesOnce(t *testing.T) {
	ctx := context.Background()
	hub := store.New(testdatabase.New(t))
	now := time.Now()

	if seeded, err := seed(ctx, hub, now); err != nil || !seeded {
		t.Fatalf("seed = %v, %v; want it seeded", seeded, err)
	}
	checkPipeline(t, hub, now)
	checkJobs(t, hub)
	checkCompanies(t, hub)
	checkScreens(t, hub)

	profile, err := hub.GetOwnerProfile(ctx)
	if err != nil || !strings.Contains(profile.Body, "Senior iOS engineer") {
		t.Errorf("profile = %q, %v; want it filled in", profile.Body, err)
	}
	if entries, err := hub.CountConfirmedProfileEntries(ctx); err != nil || entries != len(seededEntries) {
		t.Errorf("confirmed entries = %d, %v; want %d", entries, err, len(seededEntries))
	}

	// A second run adds nothing.
	if seeded, err := seed(ctx, hub, now); err != nil || seeded {
		t.Fatalf("second seed = %v, %v; want nothing seeded", seeded, err)
	}
	checkPipeline(t, hub, now)
	checkJobs(t, hub)
	checkCompanies(t, hub)
	checkScreens(t, hub)
}

// TestEveryPostingIsMarkdownItsFactsQuote checks each seeded posting is
// Markdown with headings, items and bold, as a board stores it, and its
// facts were read with evidence the hub keeps: each quote is in the
// posting's words, and some run across its bold. The values flatten as the
// flat facts were, so the fit judges them alike.
func TestEveryPostingIsMarkdownItsFactsQuote(t *testing.T) {
	acrossBold := 0
	for _, company := range seededCompanies {
		for _, job := range company.jobs {
			posting := describeJob(company, job)
			for _, marker := range []string{"\n### ", "\n- ", "**"} {
				if !strings.Contains(posting, marker) {
					t.Errorf("%s posting has no %q:\n%s", job.title, marker, posting)
				}
			}
			raw, err := json.Marshal(readJobFacts(job))
			if err != nil {
				t.Fatal(err)
			}
			values, evidence, err := store.FlattenJobFacts(raw)
			if err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{
				"location_restriction": `"` + job.location + `"`, "seniority": `"` + job.seniority + `"`, "contract_type": `"Full-time"`,
			} {
				if got := string(values[key]); got != want {
					t.Errorf("%s %s = %s; want %s", job.title, key, got, want)
				}
			}
			quoted := []string{"location_restriction", "seniority", "contract_type", "languages"}
			if job.yearsAsked > 0 {
				quoted = append(quoted, "years_of_experience")
			}
			words := strings.Join(strings.Fields(textextract.ConvertMarkdownToText(posting)), " ")
			for _, key := range quoted {
				quote := evidence[key]
				if quote == "" || !strings.Contains(words, quote) {
					t.Errorf("%s %s quotes %q; want words of its posting:\n%s", job.title, key, quote, words)
				}
				if !strings.Contains(posting, quote) {
					acrossBold++
				}
			}
		}
	}
	if acrossBold == 0 {
		t.Error("no quote runs across bold")
	}

	// The first job in Decide asks for years, quoted across bold: 12, more
	// than the owner's roles span, so its Experience row fails.
	if patientApp := seededCompanies[3].jobs[1]; patientApp.title != "Senior iOS Engineer, Patient App" || patientApp.yearsAsked != 12 {
		t.Errorf("Copperleaf Health's second job = %s asking %d years; want the Patient App asking 12", patientApp.title, patientApp.yearsAsked)
	}
}

// checkPipeline checks the board: three overdue Applied cards nobody
// answered, with three people on file, one and none to write to, the best
// of them first; an Applied card not yet due; and one card in each of
// Saved, Interviewing, due today, and Closed.
func checkPipeline(t *testing.T, hub *store.Store, now time.Time) {
	t.Helper()
	ctx := context.Background()
	cards, err := hub.ListPipelineCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	phases, err := hub.ListPipelinePhases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	phaseNames := map[string]string{}
	for _, phase := range phases {
		phaseNames[phase.ID.String()] = phase.Name
	}
	inPhase := map[string]int{}
	byCompany := map[string]store.PipelineCard{}
	var overdue []string
	for _, card := range cards {
		phase := phaseNames[card.Application.PhaseID.String()]
		inPhase[phase]++
		byCompany[*card.CompanyName] = card
		if phase == "Applied" && card.Application.ContactedAt == nil && card.FollowUpDueAt != nil && card.FollowUpDueAt.Before(now) {
			overdue = append(overdue, *card.CompanyName)
		}
	}
	if want := map[string]int{"Applied": 4, "Saved": 1, "Interviewing": 1, "Closed": 1}; !maps.Equal(inPhase, want) {
		t.Errorf("cards by phase = %v; want %v", inPhase, want)
	}
	slices.Sort(overdue)
	if want := []string{"Brightwater Labs", "Larkspur Freight", "Quillfeather Systems"}; !slices.Equal(overdue, want) {
		t.Errorf("overdue unanswered Applied cards = %v; want %v", overdue, want)
	}

	for company, want := range map[string][]string{
		"Brightwater Labs":     {"Dana Whitfield", "Priya Castellanos", "Tomasz Ellery"},
		"Quillfeather Systems": {"Ines Marchetti"},
		"Larkspur Freight":     nil,
	} {
		people, err := hub.ListRelatedPeople(ctx, byCompany[company].Application.CompanyID)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, person := range people {
			if person.Relation != store.RelationContact {
				t.Errorf("%s at %s is a %s; want a contact", person.Name, company, person.Relation)
			}
			names = append(names, person.Name)
		}
		slices.Sort(names)
		if !slices.Equal(names, want) {
			t.Errorf("people at %s = %v; want %v", company, names, want)
		}
	}
	// The Mac app ranks the hiring manager first: the card offers "Write to
	// Dana Whitfield".
	brightwater := seededCompanies[0]
	if brightwater.people[0].relevance != "hiring_manager" || brightwater.people[0].name != "Dana Whitfield" {
		t.Errorf("Brightwater Labs' first person = %+v; want the hiring manager, Dana Whitfield", brightwater.people[0])
	}

	if card := byCompany["Copperleaf Health"]; card.FollowUpDueAt == nil || !card.FollowUpDueAt.After(now) {
		t.Errorf("Copperleaf Health card = %+v; want its follow-up still to come", card)
	}
	if card := byCompany["Orchard Mesa Analytics"]; card.Application.ContactedAt == nil || card.FollowUpDueAt == nil ||
		card.FollowUpDueAt.Sub(now).Abs() > 2*time.Hour {
		t.Errorf("Orchard Mesa Analytics card = %+v; want heard back, its follow-up falling due about now", card)
	}
	if card := byCompany["Nimbleforge"]; card.Application.ClosedReason != "Role filled" {
		t.Errorf("Nimbleforge card = %+v; want closed as Role filled", card)
	}
}

// checkJobs checks the Jobs page's scopes all have jobs, every job with its
// facts and a brief that isn't stale.
func checkJobs(t *testing.T, hub *store.Store) {
	t.Helper()
	ctx := context.Background()
	counts := map[string]int{}
	for _, status := range []string{store.JobStatusOpen, store.JobStatusLater, store.JobStatusClosed, store.JobStatusDismissed, store.JobStatusAll} {
		items, total, err := hub.ListJobs(ctx, store.JobFilter{Status: status, Limit: 500})
		if err != nil {
			t.Fatal(err)
		}
		counts[status] = total
		for _, item := range items {
			if len(item.Facts) == 0 || item.Match == nil {
				t.Errorf("%s job %s has facts %s and match %v; want both", status, item.Job.Title, item.Facts, item.Match)
			}
			brief, err := hub.GetJobBrief(ctx, item.Job.ID)
			if err != nil || brief == nil || brief.IsStale || len(brief.CitedEntries) == 0 {
				t.Errorf("brief of %s = %+v, %v; want a fresh one citing the knowledge base", item.Job.Title, brief, err)
			}
			if host := getHost(t, item.Job.URL); !strings.HasSuffix(host, ".example") {
				t.Errorf("%s links to %s; want an .example domain", item.Job.Title, host)
			}
			checkPostingQuotes(t, hub, item.Job)
		}
	}
	want := map[string]int{
		store.JobStatusOpen: 11, store.JobStatusLater: 3, store.JobStatusClosed: 3, store.JobStatusDismissed: 2, store.JobStatusAll: 14,
	}
	if !maps.Equal(counts, want) {
		t.Errorf("jobs by scope = %v; want %v", counts, want)
	}
	if every := counts[store.JobStatusAll] + counts[store.JobStatusDismissed]; every < 12 {
		t.Errorf("%d jobs; want at least 12", every)
	}
}

// checkPostingQuotes checks the job's posting is stored as Markdown and the
// screen's facts quote its words: where they hire, the level, the contract
// and the languages.
func checkPostingQuotes(t *testing.T, hub *store.Store, job store.Job) {
	t.Helper()
	details, err := hub.GetJobDetails(context.Background(), job.ID)
	if err != nil || details.Facts == nil {
		t.Fatalf("details of %s = %+v, %v; want its facts", job.Title, details, err)
	}
	posting := details.Job.Description
	if !strings.Contains(posting, "\n### ") || !strings.Contains(posting, "**") {
		t.Errorf("%s posting = %q; want Markdown with headings and bold", job.Title, posting)
	}
	words := strings.Join(strings.Fields(textextract.ConvertMarkdownToText(posting)), " ")
	quoted := map[string]bool{}
	for _, entry := range details.Facts.Entries {
		if entry.Evidence != "" && strings.Contains(words, entry.Evidence) {
			quoted[entry.Key] = true
		}
	}
	for _, key := range []string{"location_restriction", "seniority", "contract_type", "languages"} {
		if !quoted[key] {
			t.Errorf("%s %s has no quote from its posting; facts %+v", job.Title, key, details.Facts.Entries)
		}
	}
}

// checkCompanies checks every company has a board and its people, one has a
// follow-up due, and two followed companies are suggested.
func checkCompanies(t *testing.T, hub *store.Store) {
	t.Helper()
	ctx := context.Background()
	summaries, err := hub.ListCompanySummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != len(seededCompanies) {
		t.Fatalf("%d companies; want %d", len(summaries), len(seededCompanies))
	}
	people := 0
	for _, summary := range summaries {
		if len(summary.JobBoards) != 1 || summary.WatchedSince == nil || !strings.HasSuffix(summary.Company.Domain, ".example") {
			t.Errorf("%s = %+v; want one board, watched, on an .example domain", summary.Company.Name, summary)
		}
		people += summary.PeopleCount
	}
	if people < 5 {
		t.Errorf("%d people on file; want a few", people)
	}
	follows, err := hub.ListCompanyFollowsNotInHub(ctx)
	if err != nil || len(follows) != len(seededFollows) {
		t.Errorf("suggestions = %+v, %v; want %v", follows, err, seededFollows)
	}
}

// screenedJob is a job as the app reads it from a jobs list row or from its
// page: the fit's checks and the answers that could screen the owner out.
type screenedJob struct {
	Job struct {
		ID    uuid.UUID `json:"id"`
		Title string    `json:"title"`
	} `json:"job"`
	CompanyName string `json:"company_name"`
	Fit         struct {
		Checks []struct {
			Name    string `json:"name"`
			Verdict string `json:"verdict"`
		} `json:"checks"`
	} `json:"fit"`
	ScreenOut []struct {
		Name    string `json:"name"`
		Verdict string `json:"verdict"`
	} `json:"screen_out"`
}

// screen is the job's screen as its page's verdict strip reads it: "fails"
// when an answer or a check says no, else "unclear" when one isn't sure,
// else "passes". An answer without a verdict judges nothing.
func (job screenedJob) screen() string {
	var verdicts []string
	for _, answer := range job.ScreenOut {
		verdicts = append(verdicts, answer.Verdict)
	}
	for _, check := range job.Fit.Checks {
		verdicts = append(verdicts, check.Verdict)
	}
	switch {
	case slices.Contains(verdicts, "no"):
		return "fails"
	case slices.Contains(verdicts, "unclear"):
		return "unclear"
	}
	return "passes"
}

// failing names the answers and checks that say no.
func (job screenedJob) failing() []string {
	var names []string
	for _, answer := range job.ScreenOut {
		if answer.Verdict == "no" {
			names = append(names, answer.Name)
		}
	}
	for _, check := range job.Fit.Checks {
		if check.Verdict == "no" {
			names = append(names, check.Name)
		}
	}
	return names
}

// checkScreens reads the open jobs through the API as the app does: at least
// two at different companies pass the whole screen, on their rows and their
// pages alike, at least one is unclear, and the Patient App fails on
// Experience alone. Each company's count of open jobs that pass is the
// number of its open rows that do, as its inspector lists them.
func checkScreens(t *testing.T, hub *store.Store) {
	t.Helper()
	routes := http.NewServeMux()
	asOwner := func(next http.Handler) http.Handler { return next }
	api.RegisterJobRoutes(routes, hub, nil, nil, asOwner)
	api.RegisterCompanyRoutes(routes, hub, nil, asOwner)
	get := func(path string, response any) {
		t.Helper()
		recorder := httptest.NewRecorder()
		routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, recorder.Code, recorder.Body)
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), response); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
	}

	var list struct {
		Jobs []screenedJob `json:"jobs"`
	}
	get("/v1/jobs?status=open&limit=500", &list)
	byScreen := map[string][]string{}
	passingAt := map[string]int{}
	for _, row := range list.Jobs {
		var page screenedJob
		get("/v1/jobs/"+row.Job.ID.String(), &page)
		if row.screen() != page.screen() {
			t.Errorf("%s row reads %s, its page %s", row.Job.Title, row.screen(), page.screen())
		}
		byScreen[page.screen()] = append(byScreen[page.screen()], page.Job.Title)
		if page.screen() == "passes" {
			passingAt[row.CompanyName]++
		}
		if page.Job.Title == "Senior iOS Engineer, Patient App" && !slices.Equal(page.failing(), []string{"Experience"}) {
			t.Errorf("the Patient App fails on %v; want Experience alone", page.failing())
		}
	}
	if len(passingAt) < 2 || len(byScreen["unclear"]) == 0 || !slices.Contains(byScreen["fails"], "Senior iOS Engineer, Patient App") {
		t.Errorf("open jobs by screen = %v; want two or more passing at different companies, one unclear, the Patient App failing", byScreen)
	}

	var companies struct {
		Companies []struct {
			Company struct {
				ID   uuid.UUID `json:"id"`
				Name string    `json:"name"`
			} `json:"company"`
			FittingJobs int     `json:"fitting_jobs"`
			BestMatch   *string `json:"best_match"`
		} `json:"companies"`
	}
	get("/v1/companies", &companies)
	for _, company := range companies.Companies {
		var inspector struct {
			Jobs []screenedJob `json:"jobs"`
		}
		get("/v1/jobs?status=open&company_id="+company.Company.ID.String(), &inspector)
		passing := 0
		for _, row := range inspector.Jobs {
			if row.screen() == "passes" {
				passing++
			}
		}
		if company.FittingJobs != passing || passing != passingAt[company.Company.Name] {
			t.Errorf("%s counts %d open jobs that pass; its inspector lists %d, the jobs list %d",
				company.Company.Name, company.FittingJobs, passing, passingAt[company.Company.Name])
		}
		if company.FittingJobs > 0 && company.BestMatch == nil {
			t.Errorf("%s has %d open jobs that pass and no best match", company.Company.Name, company.FittingJobs)
		}
	}
}

func getHost(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Hostname()
}
