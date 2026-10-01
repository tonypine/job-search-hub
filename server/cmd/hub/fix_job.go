package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tonypine/job-search-hub/server/agents/jobfixer"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// fixJob runs the job fixer on a job with the owner's note on what's wrong:
// the agent corrects the job's details through update_job. A fix that
// changes nothing fails with the agent's reason.
func fixJob(ctx context.Context, config cliConfig, jobID, note string, options triageOptions, out io.Writer) error {
	var started startedRun
	request := map[string]string{"kind": store.AgentRunKindJobFix, "input": jobID, "note": note}
	if err := postJSON(ctx, config, "/v1/agent-runs", request, &started); err != nil {
		return err
	}
	fmt.Fprintf(out, "Fixing job %s (agent run %s, prompt version %d)\n", jobID, started.AgentRun.ID, *started.AgentRun.AgentPromptVersion)

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
	var result jobfixer.Result
	if err := json.Unmarshal(outcome.Result, &result); err != nil {
		return fmt.Errorf("read the agent's result: %w", err)
	}
	if len(result.FixedFields) == 0 {
		return errors.New(result.Summary)
	}
	fmt.Fprintf(out, "\n%s\n", result.Summary)
	return nil
}
