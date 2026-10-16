package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// sessionPromptKinds are the prompts that start a Claude session or draft a
// message in one, not runs the hub records.
var sessionPromptKinds = []string{
	store.AgentPromptKindCompanySession, store.AgentPromptKindJobSession, store.AgentPromptKindOutreachDraft,
	store.AgentPromptKindRecruiterReply, store.AgentPromptKindProfileAudit, store.AgentPromptKindProfileInterview,
}

func listRunKinds(t *testing.T) (groups []store.RunGroup, kinds []store.RunKind) {
	t.Helper()
	routes := http.NewServeMux()
	api.RegisterRunKindRoutes(routes, func(next http.Handler) http.Handler { return next })
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	response, err := http.Get(server.URL + "/v1/run-kinds")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var listed struct {
		Groups []store.RunGroup `json:"groups"`
		Kinds  []store.RunKind  `json:"kinds"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("run kinds: %d %v", response.StatusCode, err)
	}
	return listed.Groups, listed.Kinds
}

func TestRunKindsListEveryKindOnceWithItsGroupAndSubject(t *testing.T) {
	groups, kinds := listRunKinds(t)
	groupIDs := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.Title == "" {
			t.Errorf("group %s has no title", group.ID)
		}
		groupIDs = append(groupIDs, group.ID)
	}
	if !slices.Equal(groupIDs, []string{"jobs", "companies", "mail_and_messages", "you", "sources"}) {
		t.Errorf("groups = %v", groupIDs)
	}
	subjectTypes := []string{"job", "company", "mail_message", "linkedin_conversation", "profile", "none"}
	engines := []string{"model_call", "claude_one_shot", "claude_agent"}
	seen := map[string]bool{}
	for _, kind := range kinds {
		if seen[kind.ID] {
			t.Errorf("kind %s is listed twice", kind.ID)
		}
		seen[kind.ID] = true
		if kind.Title == "" || kind.Description == "" || !slices.Contains(groupIDs, kind.Group) ||
			!slices.Contains(subjectTypes, kind.SubjectType) || !slices.Contains(engines, kind.Engine) {
			t.Errorf("kind = %+v", kind)
		}
	}

	byID := map[string]store.RunKind{}
	for _, kind := range kinds {
		byID[kind.ID] = kind
	}
	for id, want := range map[string]store.RunKind{
		"job_brief":      {Group: "jobs", SubjectType: "job", Engine: "model_call", Automatic: true},
		"job_brief_full": {Group: "jobs", SubjectType: "job", Engine: "claude_one_shot", Automatic: true},
		"company_triage": {Group: "companies", SubjectType: "company", Engine: "claude_agent"},
		"mail_triage":    {Group: "mail_and_messages", SubjectType: "mail_message", Engine: "model_call", Automatic: true},
		"profile_seed":   {Group: "you", SubjectType: "profile", Engine: "claude_agent"},
		"hiring_thread":  {Group: "sources", SubjectType: "none", Engine: "model_call", Automatic: true},
	} {
		got := byID[id]
		if got.Group != want.Group || got.SubjectType != want.SubjectType || got.Engine != want.Engine || got.Automatic != want.Automatic {
			t.Errorf("kind %s = %+v; want %+v", id, got, want)
		}
	}
}

// Every run the hub records is of a prompt's kind, or a full brief: a new
// prompt is either a kind of run, and in the catalog, or a session's.
func TestEveryKindThatRecordsRunsIsInTheCatalog(t *testing.T) {
	_, kinds := listRunKinds(t)
	catalog := map[string]bool{}
	for _, kind := range kinds {
		catalog[kind.ID] = true
	}
	recorded := []string{store.RunKindJobBriefFull, store.AgentRunKindCompanyTriage, store.AgentRunKindJobFinder, store.AgentRunKindJobFix,
		store.AgentRunKindProfileSeed}
	recorded = append(recorded, store.RoutedTaskKinds...)
	for _, kind := range store.AgentPromptKinds {
		if !slices.Contains(sessionPromptKinds, kind) {
			recorded = append(recorded, kind)
		}
	}
	for _, kind := range recorded {
		if !catalog[kind] {
			t.Errorf("runs of kind %s are recorded, but the catalog lacks it", kind)
		}
	}
	for _, kind := range sessionPromptKinds {
		if catalog[kind] {
			t.Errorf("%s starts a session, not a run", kind)
		}
	}
}
