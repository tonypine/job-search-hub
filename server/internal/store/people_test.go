package store_test

import (
	"context"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestAPersonsEmailIsStoredAndFilledInLater(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	input := store.PersonInput{CompanyID: company.ID, Name: "Ada Lovelace", Relevance: "recruiter", SourceURL: "https://acme.com/team"}

	person, created, err := hub.AddPerson(ctx, owner, input)
	if err != nil || !created || person.Email != "" {
		t.Fatalf("add = %+v, %v, %v", person, created, err)
	}
	input.Email = " Ada@Acme.com "
	person, created, err = hub.AddPerson(ctx, owner, input)
	if err != nil || created || person.Email != "ada@acme.com" {
		t.Fatalf("add again with an email = %+v, %v, %v; want the email filled in", person, created, err)
	}
	input.Email = "other@acme.com"
	if person, _, _ = hub.AddPerson(ctx, owner, input); person.Email != "ada@acme.com" {
		t.Fatalf("email = %q; a known email is never replaced", person.Email)
	}

	input.Email = "not-an-email"
	input.Name = "Bob"
	if _, _, err := hub.AddPerson(ctx, owner, input); err == nil {
		t.Fatal("an email without an @ should be refused")
	}
	directory, _ := hub.GetMailDirectory(ctx)
	if len(directory.PeopleWithEmail) != 1 || len(directory.Companies) != 1 {
		t.Fatalf("directory = %+v", directory)
	}
}
