// Command dev-server-seed migrates the database in HUB_DATABASE_URL and puts a
// made-up job search in it: the hub dev-server.sh starts for Symphony's QA
// pass. The Pipeline board gets cards past their follow-up with nobody's
// answer, so an overdue card offers its second route: one whose company has
// three people on file, one with one, one with nobody. A database whose board
// already has cards is left as it is.
package main

import (
	"context"
	"fmt"
	"os"
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
	if err := run(context.Background(), url); err != nil {
		fmt.Fprintln(os.Stderr, "dev-server-seed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, url string) error {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}
	seeded, err := seed(ctx, store.New(pool), time.Now())
	if err != nil {
		return err
	}
	if seeded {
		fmt.Println("Seeded the Pipeline board.")
	} else {
		fmt.Println("The Pipeline board already has cards; seeded nothing.")
	}
	return nil
}

// seededPerson is someone at a seeded company, the closest to the hire first.
type seededPerson struct {
	name, role, relevance string
}

// seededCard is one card of the seeded board: a job at a company, in a phase
// it entered daysInPhase days before the seed ran.
type seededCard struct {
	company, domain, title string
	phase                  string
	daysInPhase            int
	closedReason           string
	people                 []seededPerson
}

// seededBoard is the board QA judges. Applied asks for a follow-up after 7
// days, so the Applied cards entered 8 or more days ago are overdue, and with
// nobody's answer they offer the next person to write to.
var seededBoard = []seededCard{
	{company: "Globex", domain: "globex.example", title: "Senior iOS Engineer", phase: "Applied", daysInPhase: 12, people: []seededPerson{
		{name: "Alex Kim", role: "Engineering Manager, Mobile", relevance: "hiring_manager"},
		{name: "Jordan Lee", role: "Staff Engineer", relevance: "engineering_lead"},
		{name: "Sam Rivera", role: "Talent Partner", relevance: "recruiter"},
	}},
	{company: "Initech", domain: "initech.example", title: "Platform Engineer", phase: "Applied", daysInPhase: 10, people: []seededPerson{
		{name: "Riley Chen", role: "Head of Platform", relevance: "hiring_manager"},
	}},
	{company: "Vandelay Industries", domain: "vandelay.example", title: "Backend Engineer", phase: "Applied", daysInPhase: 9},
	{company: "Hooli", domain: "hooli.example", title: "Mobile Engineer", phase: "Applied", daysInPhase: 2, people: []seededPerson{
		{name: "Casey Morgan", role: "Engineering Director", relevance: "hiring_manager"},
	}},
	{company: "Umbrella", domain: "umbrella.example", title: "Swift Engineer", phase: "Saved"},
	{company: "Soylent", domain: "soylent.example", title: "Senior Software Engineer", phase: "Interviewing", daysInPhase: 1},
	{company: "Acme", domain: "acme.example", title: "iOS Engineer", phase: "Closed", daysInPhase: 20, closedReason: "Role filled"},
}

// seed puts seededBoard in an empty hub as of now, and reports whether it
// did: a board that already has cards keeps them, and gets nothing.
func seed(ctx context.Context, hub *store.Store, now time.Time) (bool, error) {
	cards, err := hub.ListPipelineCards(ctx)
	if err != nil {
		return false, err
	}
	if len(cards) > 0 {
		return false, nil
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
	for index, card := range seededBoard {
		phaseID, ok := phaseIDs[card.phase]
		if !ok {
			return false, fmt.Errorf("the board has no %s phase", card.phase)
		}
		company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: card.company, Domain: card.domain})
		if err != nil {
			return false, fmt.Errorf("add %s: %w", card.company, err)
		}
		for _, person := range card.people {
			if _, _, err := hub.AddPerson(ctx, owner, store.PersonInput{
				CompanyID: company.ID, Name: person.name, RoleTitle: person.role, Relevance: person.relevance,
				SourceURL: "https://" + card.domain + "/team",
			}); err != nil {
				return false, fmt.Errorf("add %s at %s: %w", person.name, card.company, err)
			}
		}
		job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{
			CompanyID: &company.ID, Title: card.title, URL: fmt.Sprintf("https://%s/careers/%d", card.domain, index+1), Location: "Remote",
		})
		if err != nil {
			return false, fmt.Errorf("add the %s job at %s: %w", card.title, card.company, err)
		}
		application, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})
		if err != nil {
			return false, fmt.Errorf("put the %s job at %s on the board: %w", card.title, card.company, err)
		}
		enteredAt := now.AddDate(0, 0, -card.daysInPhase)
		if _, err := hub.MoveApplicationAsOf(ctx, owner, application.ID, phaseID, card.closedReason, enteredAt, ""); err != nil {
			return false, fmt.Errorf("move the %s card to %s: %w", card.company, card.phase, err)
		}
	}
	return true, nil
}
