package mcptools_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestAgentWritesAreAttributedToTheirRun(t *testing.T) {
	hub := startHub(t)
	run, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	session := connect(t, hub, token)

	created := callTool[createdCompany](t, session, "create_company", map[string]any{
		"name": "Stripe", "domain": "stripe.com", "source_url": "https://stripe.com/about",
	})

	var actorKind string
	var agentRunID uuid.UUID
	err := hub.pool.QueryRow(context.Background(),
		`SELECT actor_kind, agent_run_id FROM changes WHERE entity_id = $1`, created.Company.ID).Scan(&actorKind, &agentRunID)
	if err != nil || actorKind != "agent_run" || agentRunID != run.ID {
		t.Fatalf("actor_kind=%q agent_run_id=%v err=%v, want agent_run and %v", actorKind, agentRunID, err, run.ID)
	}
}

func TestAgentsCannotChangeTheWatchList(t *testing.T) {
	hub := startHub(t)
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)
	created := callTool[createdCompany](t, agent, "create_company", map[string]any{"name": "Stripe", "domain": "stripe.com"})
	companyID := map[string]any{"company_id": created.Company.ID}

	for _, tool := range []string{"add_to_watch_list", "remove_from_watch_list"} {
		if text := callFailingTool(t, agent, tool, companyID); !strings.Contains(text, "only the owner") {
			t.Errorf("%s error = %q", tool, text)
		}
	}
	if listed := callTool[watchList](t, agent, "list_watch_list", map[string]any{}); len(listed.Companies) != 0 {
		t.Fatalf("watch list = %+v, want nothing written", listed)
	}
	var entries int
	if err := hub.pool.QueryRow(context.Background(), `SELECT count(*) FROM watch_list_entries`).Scan(&entries); err != nil || entries != 0 {
		t.Fatalf("watch list entries = %d, err = %v", entries, err)
	}
}

func TestFinishedAndExpiredRunTokensAreRefused(t *testing.T) {
	hub := startHub(t)
	finished, finishedToken := startAgentRun(t, hub, time.Now().Add(time.Hour))
	if _, err := hub.store.FinishAgentRun(context.Background(), finished.ID, store.AgentRunOutcome{Status: store.AgentRunSucceeded}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	_, expiredToken := startAgentRun(t, hub, time.Now().Add(-time.Minute))

	for name, token := range map[string]string{"finished": finishedToken, "expired": expiredToken} {
		request, _ := http.NewRequest(http.MethodPost, hub.url, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s run's token: status %d, want 401", name, response.StatusCode)
		}
	}
}
