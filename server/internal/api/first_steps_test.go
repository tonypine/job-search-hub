package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

func serveFirstSteps(t *testing.T) (*store.Store, string) {
	t.Helper()
	hub := store.New(testdatabase.New(t))
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	routes := http.NewServeMux()
	api.RegisterFirstStepRoutes(routes, hub, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return hub, server.URL
}

func getFirstSteps(t *testing.T, url string) map[string]api.FirstStep {
	t.Helper()
	status, body := send(t, http.MethodGet, url+"/v1/first-steps", ownerToken, "")
	var served struct {
		Steps []api.FirstStep `json:"steps"`
	}
	if err := json.Unmarshal(body, &served); status != http.StatusOK || err != nil {
		t.Fatalf("first steps: %d %s", status, body)
	}
	steps := map[string]api.FirstStep{}
	for _, step := range served.Steps {
		steps[step.Kind] = step
	}
	return steps
}

// addConfirmedEntries adds count entries and confirms them as the owner.
func addConfirmedEntries(t *testing.T, hub *store.Store, count int) {
	t.Helper()
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	ids := make([]uuid.UUID, count)
	for index := range ids {
		entry, err := hub.SaveProfileEntry(ctx, owner, nil, store.ProfileEntryInput{Kind: store.ProfileEntrySkill, Title: "Skill " + strconv.Itoa(index), Source: store.ProfileSourceOwner})
		if err != nil {
			t.Fatal(err)
		}
		ids[index] = entry.ID
	}
	if _, err := hub.ConfirmProfileEntries(ctx, owner, ids); err != nil {
		t.Fatal(err)
	}
}

func TestTheInterviewStepShowsUntilFiveEntriesAreConfirmed(t *testing.T) {
	hub, url := serveFirstSteps(t)

	addConfirmedEntries(t, hub, 4)
	interview, shown := getFirstSteps(t, url)[api.FirstStepProfileInterview]
	if !shown || interview.ConfirmedEntries != 4 {
		t.Fatalf("interview = %+v, %v; want it shown with 4 confirmed", interview, shown)
	}

	addConfirmedEntries(t, hub, 1)
	if _, shown := getFirstSteps(t, url)[api.FirstStepProfileInterview]; shown {
		t.Error("the interview step stayed after the fifth confirmed entry")
	}
}

func TestTheTailoredCVStepShowsOnceTheProfileIsConfirmedAndNoCVIsDrafted(t *testing.T) {
	hub, url := serveFirstSteps(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}

	if _, shown := getFirstSteps(t, url)[api.FirstStepTailoredCV]; shown {
		t.Fatal("the tailored CV step showed before five entries were confirmed")
	}
	addConfirmedEntries(t, hub, 5)
	if _, shown := getFirstSteps(t, url)[api.FirstStepTailoredCV]; !shown {
		t.Fatal("the tailored CV step didn't show with five confirmed entries and no CV")
	}

	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: "https://example.com/jobs/1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveTailoredCV(ctx, owner, job.ID, resume.Resume{Basics: resume.Basics{Name: "Ada Example"}}, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if _, shown := getFirstSteps(t, url)[api.FirstStepTailoredCV]; shown {
		t.Error("the tailored CV step stayed after a CV was drafted")
	}
}

func TestTheComparisonStepShowsWhileAFieldHasNoVerdict(t *testing.T) {
	hub, url := serveFirstSteps(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: "https://example.com/jobs/1", Description: "Go."})
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := hub.CreateComparison(ctx, owner, store.NewComparison{
		TaskKind: store.AgentPromptKindJobFacts, Title: "Key against local", JobIDs: []uuid.UUID{job.ID},
		Stacks: []store.ComparisonStack{{Label: "Key", Source: store.ComparisonSourceImported}, {Label: "Local", Source: store.ComparisonSourceImported}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, stack := range comparison.Stacks {
		answer := store.ComparisonAnswer{StackID: stack.ID, JobID: job.ID, Answer: json.RawMessage(`{"stack":{"value":"Go"},"remote":true}`)}
		if err := hub.SaveComparisonAnswer(ctx, answer); err != nil {
			t.Fatal(err)
		}
	}

	if _, shown := getFirstSteps(t, url)[api.FirstStepModelComparison]; shown {
		t.Fatal("the comparison step showed while the comparison was running")
	}
	if err := hub.FinishComparison(ctx, comparison.ID); err != nil {
		t.Fatal(err)
	}
	step, shown := getFirstSteps(t, url)[api.FirstStepModelComparison]
	if !shown || step.ComparisonID == nil || *step.ComparisonID != comparison.ID || step.ComparisonTitle != "Key against local" || step.UnjudgedFields != 2 {
		t.Fatalf("comparison step = %+v, %v; want this comparison with 2 fields to judge", step, shown)
	}

	judge := func(field string) {
		verdict := store.ComparisonVerdict{StackID: comparison.Stacks[1].ID, JobID: job.ID, Field: field, Verdict: "right"}
		if err := hub.SaveComparisonVerdict(ctx, comparison.ID, verdict); err != nil {
			t.Fatal(err)
		}
	}
	judge("stack.value")
	if step := getFirstSteps(t, url)[api.FirstStepModelComparison]; step.UnjudgedFields != 1 {
		t.Fatalf("comparison step = %+v, want 1 field left to judge", step)
	}
	judge("remote")
	if _, shown := getFirstSteps(t, url)[api.FirstStepModelComparison]; shown {
		t.Error("the comparison step stayed after every field had a verdict")
	}
}
