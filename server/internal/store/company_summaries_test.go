package store_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestACompanyRowNamesThePeopleYouKnowAndWhereYourApplicationStands(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	acme, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.example"})
	zeta, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Zeta", Domain: "zeta.example"})

	// Four connections at Acme; never talked with, the longest connected
	// come first, and the row names three.
	var connections []store.NewConnection
	for index, name := range []string{"Dana", "Ada", "Cleo", "Bea"} {
		connectedOn := time.Date(2015+index, time.January, 1, 0, 0, 0, 0, time.UTC)
		connections = append(connections, store.NewConnection{
			FirstName: name, LastName: "Example", ProfileURL: "https://www.linkedin.com/in/" + name + "-example",
			CompanyName: "Acme", Position: "Engineer", ConnectedOn: &connectedOn,
		})
	}
	if _, err := hub.ImportConnections(ctx, owner, connections); err != nil {
		t.Fatal(err)
	}

	// Three applications at Acme: one in Applied, one closed and one
	// dismissed after it. Only the one in Applied is open.
	phases, _ := hub.ListPipelinePhases(ctx)
	byName := map[string]store.PipelinePhase{}
	for _, phase := range phases {
		byName[phase.Name] = phase
	}
	addApplication := func(path string) store.Application {
		job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &acme.ID, Title: "Engineer " + path, URL: "https://acme.example/jobs/" + path})
		if err != nil {
			t.Fatal(err)
		}
		application, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID})
		if err != nil {
			t.Fatal(err)
		}
		return application
	}
	applied := addApplication("1")
	if _, err := hub.MoveApplication(ctx, owner, applied.ID, byName["Applied"].ID, ""); err != nil {
		t.Fatal(err)
	}
	closed := addApplication("2")
	if _, err := hub.MoveApplication(ctx, owner, closed.ID, byName["Closed"].ID, "Rejected."); err != nil {
		t.Fatal(err)
	}
	dismissed := addApplication("3")
	if _, err := hub.DismissApplication(ctx, owner, dismissed.ID, ""); err != nil {
		t.Fatal(err)
	}

	summaries, err := hub.ListCompanySummaries(ctx)
	if err != nil || len(summaries) != 2 {
		t.Fatalf("summaries = %+v, %v", summaries, err)
	}
	acmeRow, zetaRow := summaries[0], summaries[1]
	if acmeRow.Company.ID != acme.ID || acmeRow.ConnectionCount != 4 ||
		!slices.Equal(acmeRow.KnownPeople, []string{"Dana Example", "Ada Example", "Cleo Example"}) {
		t.Fatalf("Acme's people = %d %q; want 4, the three connected longest named", acmeRow.ConnectionCount, acmeRow.KnownPeople)
	}
	if acmeRow.ApplicationPhase == nil || *acmeRow.ApplicationPhase != "Applied" {
		t.Fatalf("Acme's phase = %v; want Applied, its one open application", acmeRow.ApplicationPhase)
	}
	if zetaRow.Company.ID != zeta.ID || zetaRow.KnownPeople == nil || len(zetaRow.KnownPeople) != 0 || zetaRow.ApplicationPhase != nil {
		t.Fatalf("Zeta = %+v; want no one known and no application", zetaRow)
	}
}
