package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

// The database the server owns sorts text by code point, uppercase before
// lowercase. The lists the owner reads by name sort it ignoring case, as
// Docker's en_US Postgres did.
var mixedCaseNames = []string{"beta", "Zeta", "acme", "Gamma"}

const wantedNameOrder = "acme,beta,Gamma,Zeta"

func assertNameOrder(t *testing.T, list string, names []string) {
	t.Helper()
	if got := strings.Join(names, ","); got != wantedNameOrder {
		t.Fatalf("%s = %s, want %s", list, got, wantedNameOrder)
	}
}

func TestTheCompaniesListAndSearchSortNamesIgnoringCase(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	for _, name := range mixedCaseNames {
		if _, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: name, Domain: name + ".example"}); err != nil {
			t.Fatal(err)
		}
	}

	summaries, err := hub.ListCompanySummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, summary := range summaries {
		listed = append(listed, summary.Company.Name)
	}
	assertNameOrder(t, "the companies list", listed)

	// Every name holds an "a".
	found, err := hub.FindCompanies(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	var matched []string
	for _, company := range found {
		matched = append(matched, company.Name)
	}
	assertNameOrder(t, "the company search", matched)
}

func TestAConnectionsListSortsLastNamesIgnoringCase(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	var connections []store.NewConnection
	for _, name := range mixedCaseNames {
		connections = append(connections, store.NewConnection{FirstName: "Sam", LastName: name, ProfileURL: "https://www.linkedin.com/in/" + name + "-example", CompanyName: "Acme"})
	}
	if _, err := hub.ImportConnections(ctx, owner, connections); err != nil {
		t.Fatal(err)
	}

	listed, err := hub.ListConnectionsAtCompanyName(ctx, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	var lastNames []string
	for _, connection := range listed {
		lastNames = append(lastNames, connection.LastName)
	}
	assertNameOrder(t, "the connections at Acme", lastNames)
}

func TestMarketGapsAndEndorsedSkillsWithEqualCountsSortIgnoringCase(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	var gaps []store.MarketGap
	var endorsements []store.NewLinkedInEndorsement
	for _, name := range mixedCaseNames {
		gaps = append(gaps, store.MarketGap{Technology: name, JobCount: 3, JobIDs: []uuid.UUID{}, ComputedAt: time.Now()})
		endorsements = append(endorsements, store.NewLinkedInEndorsement{
			Direction: store.VouchedReceived, Skill: name, FirstName: "Ada", LastName: "Lovelace", ProfileURL: "https://www.linkedin.com/in/ada", Status: "accepted",
		})
	}
	if err := hub.SaveMarketGaps(ctx, gaps); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.ImportLinkedInEndorsements(ctx, owner, endorsements); err != nil {
		t.Fatal(err)
	}

	listedGaps, err := hub.ListMarketGaps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var technologies []string
	for _, gap := range listedGaps {
		technologies = append(technologies, gap.Technology)
	}
	assertNameOrder(t, "the market gaps", technologies)

	skills, err := hub.ListEndorsedSkills(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var skillNames []string
	for _, skill := range skills {
		skillNames = append(skillNames, skill.Skill)
	}
	assertNameOrder(t, "the endorsed skills", skillNames)
}

func TestJobsSeenTogetherSortTitlesIgnoringCase(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	expiresAt := time.Now().Add(48 * time.Hour)
	var postings []store.JobPosting
	for index, name := range mixedCaseNames {
		postings = append(postings, store.JobPosting{
			ExternalID: name, CompanyName: "Globex", Title: name, Location: "Worldwide",
			URL: "https://himalayas.app/companies/globex/jobs/" + string(rune('1'+index)), ExpiresAt: &expiresAt,
		})
	}
	if _, err := hub.SyncFeedJobs(ctx, owner, store.JobSourceHimalayas, postings, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET first_seen_at = '2026-10-01T12:00:00Z'"); err != nil {
		t.Fatal(err)
	}

	items, _, err := hub.ListJobs(ctx, store.JobFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, item := range items {
		titles = append(titles, item.Job.Title)
	}
	assertNameOrder(t, "the jobs list", titles)
}
