package mcptools_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestAnAgentRecordsACareersPagesRolesAndAGoneRoleCloses(t *testing.T) {
	hub := startHub(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, _ := hub.store.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if _, _, err := hub.store.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Staff Engineer", URL: "https://acme.com/manual/1", CompanyID: &company.ID}); err != nil {
		t.Fatal(err)
	}
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)
	role := func(title, path string) map[string]any {
		return map[string]any{"title": title, "url": "https://acme.com/careers/" + path, "location": "Remote", "workplace_type": "Remote",
			"description": "Build " + title + " things."}
	}

	first := callTool[store.CareersPageSyncResult](t, agent, "record_company_jobs", map[string]any{
		"company_id": company.ID, "jobs": []any{role("Frontend Engineer", "fe"), role("Designer", "design"), role("Staff engineer!", "staff")},
	})
	if first != (store.CareersPageSyncResult{Created: 2, Seen: 3, AlreadyListed: 1}) {
		t.Fatalf("first reading = %+v", first)
	}
	second := callTool[store.CareersPageSyncResult](t, agent, "record_company_jobs", map[string]any{
		"company_id": company.ID, "jobs": []any{role("Frontend Engineer", "fe")},
	})
	if second != (store.CareersPageSyncResult{Closed: 1, Seen: 1}) {
		t.Fatalf("second reading = %+v", second)
	}
	var open []string
	rows, _ := hub.pool.Query(ctx, `SELECT title FROM jobs WHERE company_id = $1 AND source = 'careers_page' AND closed_at IS NULL`, company.ID)
	for rows.Next() {
		var title string
		rows.Scan(&title)
		open = append(open, title)
	}
	if len(open) != 1 || open[0] != "Frontend Engineer" {
		t.Errorf("open careers-page roles = %v", open)
	}
	if text := callFailingTool(t, agent, "record_company_jobs", map[string]any{
		"company_id": company.ID, "jobs": []any{map[string]any{"title": "Engineer", "url": "https://www.linkedin.com/jobs/view/1/"}},
	}); !strings.Contains(text, "linkedin.com") {
		t.Errorf("a linkedin link: %q", text)
	}
}
