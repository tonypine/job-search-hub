package mcptools_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheOwnerWritesTheProfileAndAgentsOnlyReadIt(t *testing.T) {
	hub := startHub(t)
	owner := connect(t, hub, ownerToken)
	_, token := startAgentRun(t, hub, time.Now().Add(time.Hour))
	agent := connect(t, hub, token)

	if empty := callTool[store.OwnerProfile](t, owner, "get_owner_profile", map[string]any{}); empty.Body != "" {
		t.Fatalf("initial profile = %q", empty.Body)
	}
	callTool[store.OwnerProfile](t, owner, "update_owner_profile", map[string]any{"body": "# Candidate\nSenior engineer."})

	if read := callTool[store.OwnerProfile](t, agent, "get_owner_profile", map[string]any{}); read.Body != "# Candidate\nSenior engineer." {
		t.Fatalf("agent read %q", read.Body)
	}
	if text := callRefusedTool(t, agent, "update_owner_profile", map[string]any{"body": "changed"}); !strings.Contains(text, "unknown tool") {
		t.Fatalf("error = %q", text)
	}
	if after := callTool[store.OwnerProfile](t, owner, "get_owner_profile", map[string]any{}); after.Body != "# Candidate\nSenior engineer." {
		t.Fatalf("profile changed to %q", after.Body)
	}
}
