package mcptools_test

import (
	"context"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type addedPerson struct {
	Person  store.Person `json:"person"`
	Created bool         `json:"created"`
}

func TestAddPersonNeedsASource(t *testing.T) {
	hub := startHub(t)
	session := connect(t, hub, ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Acme", "domain": "acme.com"})

	for _, arguments := range []map[string]any{
		{"company_id": created.Company.ID, "name": "Ada Lovelace", "relevance": "engineering_lead"},
		{"company_id": created.Company.ID, "name": "Ada Lovelace", "relevance": "engineering_lead", "source_url": ""},
		{"company_id": created.Company.ID, "name": "Ada Lovelace", "relevance": "engineering_lead", "source_url": "acme.com/team"},
	} {
		callFailingTool(t, session, "add_person", arguments)
	}
	var people int
	if err := hub.pool.QueryRow(context.Background(), `SELECT count(*) FROM people`).Scan(&people); err != nil || people != 0 {
		t.Fatalf("people rows = %d, err = %v, want none", people, err)
	}
}

func TestAddPersonStoresThemOnTheDossierWithTheirSource(t *testing.T) {
	hub := startHub(t)
	session := connect(t, hub, ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Acme", "domain": "acme.com"})
	arguments := map[string]any{
		"company_id": created.Company.ID, "name": "Ada Lovelace", "role_title": "Engineering Manager",
		"relevance": "hiring_manager", "source_url": "https://acme.com/team",
	}

	added := callTool[addedPerson](t, session, "add_person", arguments)
	if !added.Created || added.Person.SourceURL != "https://acme.com/team" {
		t.Fatalf("add = %+v", added)
	}
	arguments["name"] = "ada lovelace"
	if again := callTool[addedPerson](t, session, "add_person", arguments); again.Created || again.Person.ID != added.Person.ID {
		t.Fatalf("second add = %+v, want the stored person", again)
	}

	dossier := callTool[store.CompanyDossier](t, session, "get_company", map[string]any{"company_id": created.Company.ID})
	if len(dossier.People) != 1 || dossier.People[0].SourceURL != "https://acme.com/team" {
		t.Fatalf("dossier people = %+v", dossier.People)
	}
	var personChanges int
	if err := hub.pool.QueryRow(context.Background(), `SELECT count(*) FROM changes WHERE entity_type = 'person'`).Scan(&personChanges); err != nil || personChanges != 1 {
		t.Fatalf("person changes = %d, err = %v, want 1", personChanges, err)
	}
}

func TestAddPersonRefusesAnUnknownRelevance(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Acme", "domain": "acme.com"})

	callFailingTool(t, session, "add_person", map[string]any{
		"company_id": created.Company.ID, "name": "Ada", "relevance": "friend", "source_url": "https://acme.com/team",
	})
}
