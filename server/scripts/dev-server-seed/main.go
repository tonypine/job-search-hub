// Command dev-server-seed migrates the database in HUB_DATABASE_URL and puts a
// made-up job search in it: the hub dev-server.sh starts for Symphony's QA
// pass. Every company, person and link in it is invented, on .example
// domains.
//
// The Pipeline board has Applied cards past their follow-up that nobody
// answered, at a company with three people on file, one with one and one
// with nobody, so each overdue card offers its next route; an Applied card
// not yet due; and cards in Saved, Interviewing (whose follow-up falls due
// today) and Closed. The Jobs page has jobs in every scope, each with facts
// and a brief; the Companies page has every company with a job board and two
// suggestions; the owner's profile, knowledge base and criteria are filled
// in. A database that already holds companies is left as it is.
//
// The briefs and facts cite the job_brief and job_facts prompts, which it
// seeds from HUB_AGENT_PROMPTS_DIR first when that is set, as the server
// does.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func main() {
	url := os.Getenv("HUB_DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "dev-server-seed: set HUB_DATABASE_URL to the dev server's database")
		os.Exit(1)
	}
	if err := run(context.Background(), url, os.Getenv("HUB_AGENT_PROMPTS_DIR")); err != nil {
		fmt.Fprintln(os.Stderr, "dev-server-seed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, url, promptsDir string) error {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}
	hub := store.New(pool)
	if promptsDir != "" {
		if _, err := hub.SeedAgentPrompts(ctx, os.DirFS(promptsDir)); err != nil {
			return fmt.Errorf("seed the agent prompts from %s: %w", promptsDir, err)
		}
	}
	seeded, err := seed(ctx, hub, time.Now())
	if err != nil {
		return err
	}
	if seeded {
		fmt.Println("Seeded a made-up job search.")
	} else {
		fmt.Println("The hub already holds companies; seeded nothing.")
	}
	return nil
}

// The scopes a seeded job ends up in, on the Jobs page.
const (
	jobOpen    = "open"
	jobLater   = "later"
	jobSkipped = "skipped"
	jobClosed  = "closed"
)

// seededPerson is someone at a seeded company.
type seededPerson struct {
	name, role, relevance string
}

// seededJob is one posting on a seeded company's board, in the scope status
// ends it in. A job with a phase is pursued, and its card entered that phase
// daysInPhase days before the seed ran.
type seededJob struct {
	slug, title, location, workplace string
	status                           string
	dismissalReason                  string
	phase                            string
	daysInPhase                      int
	closedReason                     string
	// heardBack marks the card someone at the company wrote back on.
	heardBack bool
	payMin    float64
	payMax    float64
	// technologies and seniority are its facts.
	technologies []string
	seniority    string
	// match, reason, strength and weakness are its brief.
	match, reason, strength, weakness string
}

// seededCompany is a company with its board's postings and the people on
// file there.
type seededCompany struct {
	name, domain, industry, size, summary string
	people                                []seededPerson
	jobs                                  []seededJob
}

// seededCompanies is the search QA judges. Applied asks for a follow-up
// after 7 days and Interviewing after 5, so the Applied cards entered 8 or
// more days ago are overdue, and the Interviewing one falls due today.
var seededCompanies = []seededCompany{
	{
		name: "Brightwater Labs", domain: "brightwater.example", industry: "Developer tools", size: "51-200",
		summary: "Builds a crash and performance dashboard for mobile teams.",
		people: []seededPerson{
			{name: "Dana Whitfield", role: "Engineering Manager, Mobile", relevance: "hiring_manager"},
			{name: "Priya Castellanos", role: "Staff iOS Engineer", relevance: "engineering_lead"},
			{name: "Tomasz Ellery", role: "Technical Recruiter", relevance: "recruiter"},
		},
		jobs: []seededJob{
			{slug: "senior-ios-engineer", title: "Senior iOS Engineer", location: "Remote (Americas)", workplace: "Remote", status: jobOpen,
				phase: "Applied", daysInPhase: 12, payMin: 120000, payMax: 150000, technologies: []string{"Swift", "SwiftUI", "XCTest"}, seniority: "Senior",
				match: "strong", reason: "Seven years of shipping Swift apps, and the dashboard work is the same kind of product.",
				strength: "Led a SwiftUI rewrite of a consumer app", weakness: "No public SDK work"},
			{slug: "engineering-manager-ios", title: "Engineering Manager, iOS", location: "Remote (Americas)", workplace: "Remote", status: jobOpen,
				technologies: []string{"Swift"}, seniority: "Manager",
				match: "stretch", reason: "A manager role; the profile is an individual contributor's.",
				strength: "Mentored two junior engineers", weakness: "Never managed a team"},
		},
	},
	{
		name: "Quillfeather Systems", domain: "quillfeather.example", industry: "Logistics software", size: "201-500",
		summary: "Routes deliveries for regional couriers.",
		people: []seededPerson{
			{name: "Ines Marchetti", role: "Head of Platform", relevance: "hiring_manager"},
		},
		jobs: []seededJob{
			{slug: "platform-engineer", title: "Senior Platform Engineer, Swift", location: "Remote, worldwide", workplace: "Remote", status: jobOpen,
				phase: "Applied", daysInPhase: 10, technologies: []string{"Swift", "Vapor", "Postgres"}, seniority: "Senior",
				match: "possible", reason: "Swift on the server is new ground, but the API work carries over.",
				strength: "Built the app's offline sync API", weakness: "Little server-side Swift"},
			{slug: "site-reliability-engineer", title: "Site Reliability Engineer", location: "Remote, worldwide", workplace: "Remote", status: jobLater,
				technologies: []string{"Kubernetes", "Terraform"}, seniority: "Senior",
				match: "stretch", reason: "Infrastructure work, far from the mobile background.",
				strength: "Ran the app's release pipeline", weakness: "No Kubernetes"},
		},
	},
	{
		name: "Larkspur Freight", domain: "larkspur.example", industry: "Freight marketplace", size: "11-50",
		summary: "Matches small shippers with carriers.",
		jobs: []seededJob{
			{slug: "ios-engineer-carrier-app", title: "Senior iOS Engineer, Carrier App", location: "Remote (LATAM)", workplace: "Remote", status: jobOpen,
				phase: "Applied", daysInPhase: 9, payMin: 90000, payMax: 110000, technologies: []string{"Swift", "UIKit", "MapKit"}, seniority: "Senior",
				match: "strong", reason: "A driver-facing app with maps and offline mode, both done before.",
				strength: "Shipped offline-first map features", weakness: "No logistics domain experience"},
			{slug: "staff-backend-engineer", title: "Staff Backend Engineer", location: "Remote (LATAM)", workplace: "Remote", status: jobClosed,
				technologies: []string{"Go", "Postgres"}, seniority: "Staff",
				match: "stretch", reason: "A backend role at staff level.",
				strength: "Designed the app's sync protocol", weakness: "No production Go"},
		},
	},
	{
		name: "Copperleaf Health", domain: "copperleaf.example", industry: "Digital health", size: "201-500",
		summary: "Runs a patient app for outpatient clinics.",
		people: []seededPerson{
			{name: "Rafe Lindqvist", role: "Director of Engineering", relevance: "hiring_manager"},
		},
		jobs: []seededJob{
			{slug: "senior-mobile-engineer", title: "Senior Mobile Engineer (iOS)", location: "Remote (Americas)", workplace: "Remote", status: jobOpen,
				phase: "Applied", daysInPhase: 2, payMin: 110000, payMax: 140000, technologies: []string{"Swift", "SwiftUI", "HealthKit"}, seniority: "Senior",
				match: "strong", reason: "SwiftUI and HealthKit, both in the last role.",
				strength: "Built a HealthKit sleep tracker", weakness: "No HIPAA experience"},
			{slug: "senior-ios-engineer-patient-app", title: "Senior iOS Engineer, Patient App", location: "Remote (Americas)", workplace: "Remote", status: jobOpen,
				technologies: []string{"Swift", "SwiftUI"}, seniority: "Senior",
				match: "strong", reason: "The same stack as the role already applied for.",
				strength: "Seven years of Swift", weakness: "No clinical workflows"},
			{slug: "android-engineer", title: "Android Engineer", location: "Remote (US only)", workplace: "Remote", status: jobSkipped,
				dismissalReason: "Android only", technologies: []string{"Kotlin", "Jetpack Compose"}, seniority: "Mid-level",
				match: "mismatch", reason: "An Android role, hiring in the US only.",
				strength: "Knows the product", weakness: "No Kotlin"},
		},
	},
	{
		name: "Tidewell Robotics", domain: "tidewell.example", industry: "Robotics", size: "51-200",
		summary: "Makes warehouse robots and the tablet app that runs them.",
		people: []seededPerson{
			{name: "Noor Abernathy", role: "Engineering Lead, Apps", relevance: "engineering_lead"},
		},
		jobs: []seededJob{
			{slug: "swift-engineer-operator-app", title: "Senior Swift Engineer, Operator App", location: "Remote, worldwide", workplace: "Remote", status: jobOpen,
				phase: "Saved", daysInPhase: 3, technologies: []string{"Swift", "iPadOS"}, seniority: "Senior",
				match: "possible", reason: "An iPad app for operators; robotics is new.",
				strength: "Shipped an iPad-first app", weakness: "No robotics"},
			{slug: "firmware-engineer", title: "Firmware Engineer", location: "Hybrid, Lisbon", workplace: "Hybrid", status: jobSkipped,
				dismissalReason: "Embedded C, not my stack", technologies: []string{"C", "RTOS"}, seniority: "Senior",
				match: "mismatch", reason: "Embedded firmware, on site.",
				strength: "Knows Bluetooth LE from the app side", weakness: "No embedded C"},
			{slug: "senior-mobile-engineer", title: "Senior Mobile Engineer", location: "Remote, worldwide", workplace: "Remote", status: jobLater,
				technologies: []string{"Swift", "React Native"}, seniority: "Senior",
				match: "possible", reason: "Half the work is React Native.",
				strength: "Seven years of Swift", weakness: "Little React Native"},
		},
	},
	{
		name: "Orchard Mesa Analytics", domain: "orchardmesa.example", industry: "Analytics", size: "501-1,000",
		summary: "Sells crop-yield forecasts to growers.",
		people: []seededPerson{
			{name: "Jonah Pettigrew", role: "Engineering Manager", relevance: "hiring_manager"},
			{name: "Celeste Varga", role: "Recruiter", relevance: "recruiter"},
		},
		jobs: []seededJob{
			{slug: "senior-software-engineer-ios", title: "Senior Software Engineer, iOS", location: "Remote (Americas)", workplace: "Remote", status: jobOpen,
				phase: "Interviewing", daysInPhase: 5, heardBack: true, payMin: 130000, payMax: 160000, technologies: []string{"Swift", "Swift Charts"}, seniority: "Senior",
				match: "strong", reason: "Charts-heavy SwiftUI, as in the dashboard work.",
				strength: "Built Swift Charts dashboards", weakness: "No agriculture domain"},
			{slug: "data-engineer", title: "Data Engineer", location: "Remote (Americas)", workplace: "Remote", status: jobLater,
				technologies: []string{"Python", "dbt"}, seniority: "Senior",
				match: "stretch", reason: "A data role, outside the mobile path.",
				strength: "Wrote the app's analytics events", weakness: "No dbt"},
		},
	},
	{
		name: "Nimbleforge", domain: "nimbleforge.example", industry: "Games", size: "11-50",
		summary: "Makes puzzle games for phones.",
		jobs: []seededJob{
			{slug: "ios-engineer", title: "Senior iOS Engineer, Games", location: "Remote, worldwide", workplace: "Remote", status: jobClosed,
				phase: "Closed", daysInPhase: 20, closedReason: "Role filled", technologies: []string{"Swift", "SpriteKit"}, seniority: "Senior",
				match: "possible", reason: "SpriteKit is new, the rest is familiar.",
				strength: "Seven years of Swift", weakness: "No game development"},
			{slug: "unity-developer", title: "Unity Developer", location: "Remote, worldwide", workplace: "Remote", status: jobClosed,
				technologies: []string{"Unity", "C#"}, seniority: "Mid-level",
				match: "mismatch", reason: "A Unity role.",
				strength: "Shipped games-adjacent animation work", weakness: "No Unity"},
		},
	},
}

// seededFollows are companies the owner follows on LinkedIn that the hub
// doesn't hold: the Companies page's suggestions.
var seededFollows = []string{"Saltmarsh Instruments", "Kestrel Finch Studio"}

const seededProfile = `# Avery Quinlan (made-up QA profile)

Senior iOS engineer with seven years of Swift, the last three on SwiftUI. Based in Brazil, working remotely for teams in the Americas.

## What I'm after

- Senior or Staff iOS roles on a product team, remote.
- Apps where performance and offline use matter: maps, health, dashboards.

## Strengths

- SwiftUI rewrites of large UIKit apps, done gradually.
- HealthKit, MapKit and Swift Charts in production.
- Release pipelines and crash triage.
`

// seededEntries are the knowledge base the briefs cite; the first is the
// role the others happened in.
var seededEntries = []store.ProfileEntryInput{
	{Kind: store.ProfileEntryRole, Title: "Senior iOS Engineer", Organization: "Fernhollow Apps", StartMonth: "2022-03",
		Body: "Lead iOS engineer on a sleep and fitness app.", Skills: []string{"Swift", "SwiftUI", "HealthKit"}},
	{Kind: store.ProfileEntryRole, Title: "iOS Engineer", Organization: "Pebblecrest Media", StartMonth: "2018-01", EndMonth: "2022-02",
		Body: "Built a news reader with offline maps of local stories.", Skills: []string{"Swift", "UIKit", "MapKit"}},
	{Kind: store.ProfileEntryCase, Title: "Moved the app to SwiftUI screen by screen", Outcome: "No release slipped; crash rate halved.",
		Skills: []string{"SwiftUI"}},
	{Kind: store.ProfileEntrySkill, Title: "Swift Charts dashboards", Skills: []string{"Swift Charts"}},
}

var seededCriteria = store.JobCriteria{
	Roles:                   []string{"Senior iOS Engineer", "Senior Mobile Engineer", "Senior Swift Engineer"},
	ExcludedRoleTerms:       []string{"Android", "Firmware", "Unity"},
	SearchTerms:             []string{"ios", "swift"},
	Technologies:            []string{"Swift", "SwiftUI"},
	SeniorityLevels:         []string{"Senior", "Staff"},
	HomeCountry:             "Brazil",
	EligibleLocationTerms:   []string{"LATAM", "Americas", "worldwide"},
	IneligibleLocationTerms: []string{"US only"},
}

// seed puts the made-up search in an empty hub as of now, and reports
// whether it did: a hub that already holds companies gets nothing.
func seed(ctx context.Context, hub *store.Store, now time.Time) (bool, error) {
	existing, err := hub.ListCompanySummaries(ctx)
	if err != nil {
		return false, err
	}
	if len(existing) > 0 {
		return false, nil
	}
	briefPrompt, err := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobBrief)
	if err != nil {
		return false, fmt.Errorf("read the job_brief prompt (set HUB_AGENT_PROMPTS_DIR to seed one): %w", err)
	}
	factsPrompt, err := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	if err != nil {
		return false, fmt.Errorf("read the job_facts prompt (set HUB_AGENT_PROMPTS_DIR to seed one): %w", err)
	}
	phases, err := hub.ListPipelinePhases(ctx)
	if err != nil {
		return false, err
	}
	phaseIDs := map[string]uuid.UUID{}
	for _, phase := range phases {
		phaseIDs[phase.Name] = phase.ID
	}

	owner := store.Actor{Kind: store.ActorOwner}
	entryIDs, err := seedOwner(ctx, hub, owner)
	if err != nil {
		return false, err
	}
	// The briefs are written against the knowledge base as it now is, so
	// none reads as stale.
	knowledgeHash, err := hub.GetKnowledgeHash(ctx)
	if err != nil {
		return false, err
	}

	for index, company := range seededCompanies {
		// The companies' boards were first read a few weeks back, the first
		// company's last, so it lists first on the Jobs page.
		firstSeen := now.AddDate(0, 0, -21-index)
		jobs, err := seedCompany(ctx, hub, owner, company, firstSeen, now)
		if err != nil {
			return false, err
		}
		for _, job := range company.jobs {
			jobID := jobs[job.slug]
			if err := seedJobReading(ctx, hub, jobID, job, factsPrompt.ID, briefPrompt.ID, knowledgeHash, entryIDs); err != nil {
				return false, fmt.Errorf("read the %s job at %s: %w", job.title, company.name, err)
			}
			if err := seedJobStatus(ctx, hub, owner, jobID, job, phaseIDs, now); err != nil {
				return false, fmt.Errorf("decide the %s job at %s: %w", job.title, company.name, err)
			}
		}
	}

	followedAt := now.AddDate(0, -2, 0)
	follows := make([]store.NewCompanyFollow, 0, len(seededFollows))
	for _, name := range seededFollows {
		follows = append(follows, store.NewCompanyFollow{Organization: name, FollowedAt: &followedAt})
	}
	if _, err := hub.ImportCompanyFollows(ctx, owner, follows); err != nil {
		return false, err
	}
	// The startups.gallery reader reads the list weekly: a read now keeps the
	// dev server from fetching it and adding real companies.
	if err := hub.RecordGalleryListRead(ctx, 0); err != nil {
		return false, err
	}
	return true, nil
}

// seedOwner fills in the owner's profile, knowledge base and criteria, and
// returns the entries' ids in seededEntries' order.
func seedOwner(ctx context.Context, hub *store.Store, owner store.Actor) ([]uuid.UUID, error) {
	if _, err := hub.SaveOwnerProfile(ctx, owner, seededProfile); err != nil {
		return nil, fmt.Errorf("save the profile: %w", err)
	}
	var ids []uuid.UUID
	for _, input := range seededEntries {
		input.Source = store.ProfileSourceOwner
		if input.Kind != store.ProfileEntryRole {
			input.RoleID = &ids[0]
		}
		entry, err := hub.SaveProfileEntry(ctx, owner, nil, input)
		if err != nil {
			return nil, fmt.Errorf("add the %q entry: %w", input.Title, err)
		}
		ids = append(ids, entry.ID)
	}
	if _, err := hub.ConfirmProfileEntries(ctx, owner, ids); err != nil {
		return nil, err
	}
	if _, err := hub.SaveJobCriteria(ctx, owner, seededCriteria); err != nil {
		return nil, fmt.Errorf("save the criteria: %w", err)
	}
	return ids, nil
}

// seedCompany stores the company, its people and its board with the
// board's postings, the closed ones already gone from it, and returns the
// jobs' ids by slug.
func seedCompany(ctx context.Context, hub *store.Store, owner store.Actor, company seededCompany, firstSeen, now time.Time) (map[string]uuid.UUID, error) {
	site := "https://" + company.domain
	stored, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: company.name, Domain: company.domain, WebsiteURL: site, SourceURL: site})
	if err != nil {
		return nil, fmt.Errorf("add %s: %w", company.name, err)
	}
	careers := site + "/careers"
	if _, err := hub.UpdateCompany(ctx, owner, stored.ID, store.CompanyUpdate{
		CareersURL: &careers, Industry: &company.industry, EmployeeCountRange: &company.size, Summary: &company.summary, SourceURL: site,
	}); err != nil {
		return nil, fmt.Errorf("describe %s: %w", company.name, err)
	}
	if _, _, err := hub.AddToWatchList(ctx, owner, stored.ID); err != nil {
		return nil, fmt.Errorf("watch %s: %w", company.name, err)
	}
	for _, person := range company.people {
		if _, _, err := hub.AddPerson(ctx, owner, store.PersonInput{
			CompanyID: stored.ID, Name: person.name, RoleTitle: person.role, Relevance: person.relevance, SourceURL: site + "/team",
		}); err != nil {
			return nil, fmt.Errorf("add %s at %s: %w", person.name, company.name, err)
		}
	}

	var postings, stillOpen []store.JobPosting
	for _, job := range company.jobs {
		posting := store.JobPosting{
			ExternalID: job.slug, Title: job.title, Location: job.location, WorkplaceType: job.workplace,
			URL: careers + "/" + job.slug, Description: describeJob(company, job), Raw: json.RawMessage(`{}`),
			BoardFacts: store.BoardFacts{EmploymentType: "Full-time"},
		}
		if job.payMax > 0 {
			posting.Pay = &store.Pay{Ranges: []store.PayRange{{Min: job.payMin, Max: job.payMax, Currency: "USD", Interval: "year"}}}
		}
		postings = append(postings, posting)
		if job.status != jobClosed {
			stillOpen = append(stillOpen, posting)
		}
	}
	openCount := len(stillOpen)
	board, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{
		CompanyID: stored.ID, Provider: "other", BoardToken: company.domain, BoardURL: careers, Verified: true,
		OpenPostingCount: &openCount, SourceURL: careers,
	})
	if err != nil {
		return nil, fmt.Errorf("add the board of %s: %w", company.name, err)
	}
	if _, err := hub.SyncBoardJobs(ctx, owner, board, postings, firstSeen); err != nil {
		return nil, fmt.Errorf("read the board of %s: %w", company.name, err)
	}
	if len(stillOpen) < len(postings) {
		if _, err := hub.SyncBoardJobs(ctx, owner, board, stillOpen, now.AddDate(0, 0, -1)); err != nil {
			return nil, fmt.Errorf("read the board of %s again: %w", company.name, err)
		}
	}

	listed, _, err := hub.ListJobs(ctx, store.JobFilter{CompanyID: &stored.ID, Status: store.JobStatusAll})
	if err != nil {
		return nil, err
	}
	ids := map[string]uuid.UUID{}
	for _, item := range listed {
		if item.Job.ExternalID != nil {
			ids[*item.Job.ExternalID] = item.Job.ID
		}
	}
	for _, job := range company.jobs {
		if _, ok := ids[job.slug]; !ok {
			return nil, fmt.Errorf("the board of %s has no %s job", company.name, job.title)
		}
	}
	return ids, nil
}

// describeJob writes the posting's text.
func describeJob(company seededCompany, job seededJob) string {
	return fmt.Sprintf(`About %s

%s %s

The role

We're hiring a %s (%s). You'll work with %s, and own features from design to release.

What we're looking for

- %s experience at %s level
- Care for performance, accessibility and testing
- Clear writing, in English

This posting is made up, for testing the hub.`,
		company.name, company.name, company.summary, job.title, job.location, strings.Join(job.technologies, ", "),
		strings.Join(job.technologies, ", "), strings.ToLower(job.seniority))
}

// seedJobReading saves the job's facts and its brief, as the models would
// have read them. A pursued job's brief is a full one.
func seedJobReading(ctx context.Context, hub *store.Store, jobID uuid.UUID, job seededJob, factsPromptID, briefPromptID uuid.UUID,
	knowledgeHash string, entryIDs []uuid.UUID) error {
	awaiting, err := hub.GetJobForFacts(ctx, jobID)
	if err != nil {
		return err
	}
	facts, err := json.Marshal(map[string]any{
		"summary": job.reason, "technologies": job.technologies, "seniority": job.seniority, "years_of_experience": nil,
		"location_restriction": job.location, "timezone_requirement": "not stated", "visa_sponsorship": "not stated",
		"contract_type": "Full-time", "pay_in_text": "not stated", "languages": []string{"English"},
	})
	if err != nil {
		return err
	}
	if err := hub.SaveJobFacts(ctx, store.NewJobFacts{
		JobID: jobID, PromptID: factsPromptID, Model: "qa-seed", TextHash: awaiting.TextHash, Facts: facts,
	}); err != nil {
		return err
	}
	tier := store.JobBriefTierPre
	if job.phase != "" {
		tier = store.JobBriefTierFull
	}
	return hub.SaveJobBrief(ctx, store.JobBrief{
		JobID: jobID, Tier: tier, PromptID: briefPromptID, Model: "qa-seed", Match: job.match, Reason: job.reason,
		Strengths:     []store.JobBriefPoint{{Point: job.strength, EntryIDs: []uuid.UUID{entryIDs[0], entryIDs[2]}}},
		Weaknesses:    []store.JobBriefPoint{{Point: job.weakness, EntryIDs: []uuid.UUID{}}},
		KnowledgeHash: knowledgeHash,
	})
}

// seedJobStatus records the owner's decision on the job, and moves a
// pursued job's card to its phase as of daysInPhase days ago.
func seedJobStatus(ctx context.Context, hub *store.Store, owner store.Actor, jobID uuid.UUID, job seededJob, phaseIDs map[string]uuid.UUID,
	now time.Time) error {
	switch {
	case job.status == jobLater:
		_, err := hub.DecideJob(ctx, owner, jobID, store.JobDecisionLater, "")
		return err
	case job.status == jobSkipped:
		_, err := hub.DecideJob(ctx, owner, jobID, store.JobDecisionSkip, job.dismissalReason)
		return err
	case job.phase == "":
		return nil
	}
	phaseID, ok := phaseIDs[job.phase]
	if !ok {
		return fmt.Errorf("the board has no %s phase", job.phase)
	}
	if _, err := hub.DecideJob(ctx, owner, jobID, store.JobDecisionPursue, ""); err != nil {
		return err
	}
	application, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &jobID})
	if err != nil {
		return err
	}
	enteredAt := now.AddDate(0, 0, -job.daysInPhase)
	if _, err := hub.MoveApplicationAsOf(ctx, owner, application.ID, phaseID, job.closedReason, enteredAt, ""); err != nil {
		return err
	}
	if job.heardBack {
		if _, err := hub.MarkApplicationContacted(ctx, owner, application.ID, enteredAt.Add(-time.Hour), ""); err != nil {
			return err
		}
	}
	return nil
}
