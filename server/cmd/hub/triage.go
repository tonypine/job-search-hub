package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/agents/companytriage"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// triageTimeout stops a session before its run token expires at 30 minutes.
const triageTimeout = 25 * time.Minute

type startedRun struct {
	AgentRun     store.AgentRun  `json:"agent_run"`
	Token        string          `json:"token"`
	Prompt       string          `json:"prompt"`
	ResultSchema json.RawMessage `json:"result_schema"`
}

// addCompany runs the company triage agent on company (a name, domain or URL),
// records how the run ended, and puts the company on the watch list.
func addCompany(ctx context.Context, config cliConfig, company string, options triageOptions, out io.Writer) error {
	var started startedRun
	if err := postJSON(ctx, config, "/v1/agent-runs", map[string]string{"kind": store.AgentRunKindCompanyTriage, "input": company}, &started); err != nil {
		return err
	}
	fmt.Fprintf(out, "Researching %s (agent run %s, prompt version %d)\n", company, started.AgentRun.ID, *started.AgentRun.AgentPromptVersion)

	sessionCtx, cancel := context.WithTimeout(ctx, triageTimeout)
	defer cancel()
	outcome := runTriageSession(sessionCtx, config.HubURL, started, options, out)

	// The run is recorded even when ctx was interrupted, so the finish call
	// gets a context that outlives it.
	var finished struct {
		AgentRun store.AgentRun `json:"agent_run"`
	}
	finishPath := "/v1/agent-runs/" + started.AgentRun.ID.String() + "/finish"
	if err := postJSON(context.WithoutCancel(ctx), config, finishPath, outcome, &finished); err != nil {
		return fmt.Errorf("record the run's outcome: %w", err)
	}
	if outcome.Status != store.AgentRunSucceeded {
		return errors.New("the run failed: " + outcome.Error)
	}

	var result companytriage.Result
	if err := json.Unmarshal(outcome.Result, &result); err != nil {
		return fmt.Errorf("read the agent's result: %w", err)
	}
	fmt.Fprintf(out, "\n%s\n", result.Summary)
	for _, gap := range result.Unresolved {
		fmt.Fprintf(out, "  unresolved: %s\n", gap)
	}
	if result.CompanyID == nil {
		return errors.New("the agent could not identify " + company)
	}
	companyID, err := uuid.Parse(*result.CompanyID)
	if err != nil {
		return fmt.Errorf("the agent returned an invalid company id %q", *result.CompanyID)
	}

	session, err := connectToHub(ctx, config)
	if err != nil {
		return err
	}
	defer session.Close()
	if _, err := callTool[struct{}](ctx, session, "add_to_watch_list", map[string]any{"company_id": companyID}); err != nil {
		return fmt.Errorf("put the company on the watch list: %w", err)
	}
	dossier, err := callTool[store.CompanyDossier](ctx, session, "get_company", map[string]any{"company_id": companyID})
	if err != nil {
		return err
	}
	fmt.Fprintln(out)
	printDossier(out, dossier)
	return nil
}
