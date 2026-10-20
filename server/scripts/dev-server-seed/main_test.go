package main

import (
	"context"
	"maps"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
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

func getHost(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Hostname()
}
