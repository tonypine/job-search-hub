package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tonypine/job-search-hub/server/agents/profileseed"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// seedProfile runs the agent that builds the owner's knowledge base from the
// CV, the LinkedIn export and the answers library, then records an update
// saying what it built and what needs the owner.
func seedProfile(ctx context.Context, config cliConfig, options triageOptions, out io.Writer) error {
	var started startedRun
	if err := postJSON(ctx, config, "/v1/agent-runs", map[string]string{"kind": store.AgentRunKindProfileSeed}, &started); err != nil {
		return err
	}
	fmt.Fprintf(out, "Building the knowledge base from your CV and LinkedIn (agent run %s, prompt version %d)\n",
		started.AgentRun.ID, *started.AgentRun.AgentPromptVersion)

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

	var result profileseed.Result
	if err := json.Unmarshal(outcome.Result, &result); err != nil {
		return fmt.Errorf("read the agent's result: %w", err)
	}
	fmt.Fprintf(out, "\n%s\n", result.Summary)
	for _, conflict := range result.Conflicts {
		fmt.Fprintf(out, "  to settle: %s\n", conflict)
	}
	fmt.Fprintf(out, "Entries added: %d, updated: %d\n", result.EntriesAdded, result.EntriesUpdated)

	title, body := describeSeedUpdate(result)
	if err := postJSON(ctx, config, "/v1/updates", map[string]string{"kind": "profile_seeded", "title": title, "body": body}, nil); err != nil {
		return fmt.Errorf("record the update: %w", err)
	}
	return nil
}

// describeSeedUpdate is the update a seed leaves: what it built, and each
// conflict the owner has to settle.
func describeSeedUpdate(result profileseed.Result) (string, string) {
	title := fmt.Sprintf("Built %d entries from your CV and LinkedIn", result.EntriesAdded)
	if result.EntriesAdded == 0 {
		title = "Your knowledge base already had what your CV and LinkedIn say"
	}
	lines := []string{result.Summary}
	if result.EntriesUpdated > 0 {
		lines = append(lines, fmt.Sprintf("Updated %d existing entries.", result.EntriesUpdated))
	}
	for _, conflict := range result.Conflicts {
		lines = append(lines, "To settle: "+conflict)
	}
	lines = append(lines, "Review and confirm them on the Profile page.")
	return title, strings.Join(lines, "\n")
}
