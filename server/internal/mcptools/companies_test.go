package mcptools_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type createdCompany struct {
	Company store.Company `json:"company"`
	Created bool          `json:"created"`
}

type foundCompanies struct {
	Companies []store.Company `json:"companies"`
}

func TestRequestsWithoutTheOwnerTokenAreRefused(t *testing.T) {
	hub := startHub(t)
	for _, authorization := range []string{"", "Bearer not-the-owner-token"} {
		request, _ := http.NewRequest(http.MethodPost, hub.url, strings.NewReader(`{}`))
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("authorization %q: status %d, want 401", authorization, response.StatusCode)
		}
	}
}

func TestTheOwnerSeesEveryTool(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	want := []string{
		"add_person", "add_to_watch_list", "confirm_profile_entries", "create_company", "decide_job", "delete_profile_entry", "dismiss_application", "dismiss_job", "find_companies", "find_jobs", "get_agent_prompt",
		"get_artifact_text", "get_company", "get_job_criteria", "get_owner_profile", "list_application_answers", "list_artifacts", "list_profile_entries",
		"list_watch_list", "record_company_jobs", "remove_from_watch_list", "restore_application", "restore_job", "save_application_answer", "save_profile_entry", "set_job_board",
		"set_job_company", "update_agent_prompt", "update_company", "update_job", "update_job_criteria", "update_owner_profile",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
}

func TestCreatedCompaniesRoundTripAndRecordTheOwner(t *testing.T) {
	hub := startHub(t)
	session := connect(t, hub, ownerToken)

	created := callTool[createdCompany](t, session, "create_company", map[string]any{
		"name": "Stripe", "domain": "https://www.stripe.com/", "source_url": "https://stripe.com/about", "found_via": "Board: startups.gallery",
	})
	if !created.Created || created.Company.Domain != "stripe.com" || created.Company.FoundVia != "Board: startups.gallery" {
		t.Fatalf("create = %+v", created)
	}

	byID := callTool[store.CompanyDossier](t, session, "get_company", map[string]any{"company_id": created.Company.ID}).Company
	byDomain := callTool[store.CompanyDossier](t, session, "get_company", map[string]any{"domain": "stripe.com"}).Company
	if byID.ID != created.Company.ID || byDomain.ID != created.Company.ID || byID.Name != "Stripe" {
		t.Fatalf("by id = %+v, by domain = %+v", byID, byDomain)
	}

	var actorKind string
	err := hub.pool.QueryRow(context.Background(), `SELECT actor_kind FROM changes WHERE entity_id = $1`, created.Company.ID).Scan(&actorKind)
	if err != nil || actorKind != "owner" {
		t.Fatalf("actor_kind = %q, err = %v", actorKind, err)
	}
}

func TestCreateCompanyReturnsTheStoredCompanyForAKnownDomain(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)

	first := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Stripe", "domain": "stripe.com"})
	again := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Stripe Inc", "domain": "stripe.com/jobs"})
	if again.Created || again.Company.ID != first.Company.ID {
		t.Fatalf("second create = %+v, want the first company with created=false", again)
	}
}

func TestFindAndUpdateCompanies(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Clio", "domain": "clio.com"})

	if none := callTool[foundCompanies](t, session, "find_companies", map[string]any{"query": "nothing-matches"}); len(none.Companies) != 0 {
		t.Fatalf("find with no match = %+v", none)
	}
	found := callTool[foundCompanies](t, session, "find_companies", map[string]any{"query": "cli"})
	if len(found.Companies) != 1 || found.Companies[0].ID != created.Company.ID {
		t.Fatalf("find = %+v", found)
	}

	updated := callTool[store.Company](t, session, "update_company", map[string]any{
		"company_id": created.Company.ID, "careers_url": "https://clio.com/careers", "source_url": "https://clio.com",
	})
	if updated.CareersURL != "https://clio.com/careers" || updated.Name != "Clio" {
		t.Fatalf("update = %+v", updated)
	}
}

func TestGetCompanyExplainsWhatIsMissing(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)

	if text := callFailingTool(t, session, "get_company", map[string]any{}); !strings.Contains(text, "company_id or domain") {
		t.Fatalf("error = %q", text)
	}
	if text := callFailingTool(t, session, "get_company", map[string]any{"domain": "unknown.com"}); !strings.Contains(text, "not found") {
		t.Fatalf("error = %q", text)
	}
}

func TestAnAgentsSessionListsNoOwnerTool(t *testing.T) {
	hub := startHub(t)
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)

	listed, err := agent.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	for _, ownerTool := range []string{"add_to_watch_list", "remove_from_watch_list", "update_agent_prompt", "update_job_criteria", "update_owner_profile", "save_application_answer"} {
		if names[ownerTool] {
			t.Errorf("an agent's session lists %s", ownerTool)
		}
	}
	for _, agentTool := range []string{"create_company", "get_company", "add_person", "set_job_board", "list_application_answers", "get_artifact_text"} {
		if !names[agentTool] {
			t.Errorf("an agent's session lacks %s", agentTool)
		}
	}
}
