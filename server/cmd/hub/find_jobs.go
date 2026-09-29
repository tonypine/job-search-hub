package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/tonypine/job-search-hub/server/agents/jobfinder"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// findCompanyJobs runs the job finder on a company the hub holds, given by
// its domain or id: the agent sets its job board when the hub reads it, or
// records the roles off its careers page.
func findCompanyJobs(ctx context.Context, config cliConfig, company string, options triageOptions, out io.Writer) error {
	var started startedRun
	if err := postJSON(ctx, config, "/v1/agent-runs", map[string]string{"kind": store.AgentRunKindJobFinder, "input": company}, &started); err != nil {
		return err
	}
	fmt.Fprintf(out, "Finding jobs at %s (agent run %s, prompt version %d)\n", company, started.AgentRun.ID, *started.AgentRun.AgentPromptVersion)

	sessionCtx, cancel := context.WithTimeout(ctx, triageTimeout)
	defer cancel()
	outcome := runTriageSession(sessionCtx, config.HubURL, started, options, out)
	var finished struct {
		AgentRun store.AgentRun `json:"agent_run"`
	}
	if err := postJSON(context.WithoutCancel(ctx), config, "/v1/agent-runs/"+started.AgentRun.ID.String()+"/finish", outcome, &finished); err != nil {
		return fmt.Errorf("record the run's outcome: %w", err)
	}
	if outcome.Status != store.AgentRunSucceeded {
		return errors.New("the run failed: " + outcome.Error)
	}

	var result jobfinder.Result
	if err := json.Unmarshal(outcome.Result, &result); err != nil {
		return fmt.Errorf("read the agent's result: %w", err)
	}
	fmt.Fprintf(out, "\n%s\n", result.Summary)
	for _, gap := range result.Unresolved {
		fmt.Fprintf(out, "  unresolved: %s\n", gap)
	}
	if result.JobBoard != nil {
		fmt.Fprintf(out, "Job board: %s/%s (verified: %t)\n", result.JobBoard.Provider, result.JobBoard.BoardToken, result.JobBoard.Verified)
	}
	fmt.Fprintf(out, "Roles recorded from the careers page: %d\n", result.JobsRecorded)
	var jobs struct {
		Total int `json:"total"`
	}
	if err := getJSON(ctx, config, "/v1/jobs?"+url.Values{"company_id": {result.CompanyID}, "status": {"open"}, "limit": {"1"}}.Encode(), &jobs); err == nil {
		fmt.Fprintf(out, "Open jobs at the company now: %d\n", jobs.Total)
	}
	fmt.Fprintf(out, "Company id: %s\n", result.CompanyID)
	return nil
}
