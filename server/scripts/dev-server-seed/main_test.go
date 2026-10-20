package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestTheSeedGivesOverdueCardsTheirSecondRoutesOnce(t *testing.T) {
	ctx := context.Background()
	hub := store.New(testdatabase.New(t))
	now := time.Now()

	if seeded, err := seed(ctx, hub, now); err != nil || !seeded {
		t.Fatalf("seed = %v, %v; want it seeded", seeded, err)
	}
	cards, err := hub.ListPipelineCards(ctx)
	if err != nil || len(cards) != len(seededBoard) {
		t.Fatalf("cards = %d, %v; want %d", len(cards), err, len(seededBoard))
	}
	phases, err := hub.ListPipelinePhases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	phaseNames := map[string]string{}
	for _, phase := range phases {
		phaseNames[phase.ID.String()] = phase.Name
	}
	byCompany := map[string]store.PipelineCard{}
	for _, card := range cards {
		byCompany[*card.CompanyName] = card
	}

	// The overdue Applied cards nobody answered, and who to write to next at each.
	for company, want := range map[string][]string{
		"Globex":              {"Alex Kim", "Jordan Lee", "Sam Rivera"},
		"Initech":             {"Riley Chen"},
		"Vandelay Industries": nil,
	} {
		card := byCompany[company]
		if phaseNames[card.Application.PhaseID.String()] != "Applied" || card.Application.ContactedAt != nil ||
			card.FollowUpDueAt == nil || !card.FollowUpDueAt.Before(now.AddDate(0, 0, -1)) {
			t.Errorf("%s card = %+v; want in Applied, unanswered, its follow-up days overdue", company, card)
		}
		people, err := hub.ListRelatedPeople(ctx, card.Application.CompanyID)
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
	if hooli := byCompany["Hooli"]; hooli.FollowUpDueAt == nil || !hooli.FollowUpDueAt.After(now) {
		t.Errorf("Hooli card = %+v; want its follow-up still to come", hooli)
	}
	if acme := byCompany["Acme"]; phaseNames[acme.Application.PhaseID.String()] != "Closed" || acme.Application.ClosedReason != "Role filled" {
		t.Errorf("Acme card = %+v; want closed as Role filled", acme)
	}

	if seeded, err := seed(ctx, hub, now); err != nil || seeded {
		t.Fatalf("second seed = %v, %v; want nothing seeded", seeded, err)
	}
	if again, err := hub.ListPipelineCards(ctx); err != nil || len(again) != len(seededBoard) {
		t.Fatalf("cards after the second seed = %d, %v; want %d", len(again), err, len(seededBoard))
	}
}
