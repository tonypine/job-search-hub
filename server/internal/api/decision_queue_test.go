package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheDecisionSignalsLeaveOutTheJobsWhosePagesFailTheScreen(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := service.hub.SaveJobCriteria(ctx, owner, store.JobCriteria{
		Technologies: []string{"Go"}, SeniorityLevels: []string{"Senior"}, EligibleLocationTerms: []string{"Americas"},
		WorkableTimezoneTerms: []string{"Americas"},
	}); err != nil {
		t.Fatal(err)
	}
	confirmEightYearsOfRoles(t, service)
	prompt, _ := service.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	// Both jobs pass the fit. The second asks for twelve years against the
	// owner's eight, so its page fails it on Experience, and deciding it
	// doesn't count either.
	var askingTwelve store.Job
	for index, years := range []int{6, 12} {
		job, _, err := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{
			Title: "Senior Go Engineer", Location: "Americas", URL: fmt.Sprintf("https://acme.example/jobs/%d", index),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.hub.SaveJobFacts(ctx, store.NewJobFacts{
			JobID: job.ID, PromptID: prompt.ID, Model: "test-model", TextHash: []byte{1},
			Facts: json.RawMessage(fmt.Sprintf(`{"years_of_experience":{"evidence":"%d+ years","value":%d},"timezone_requirement":"Americas hours"}`, years, years)),
		}); err != nil {
			t.Fatal(err)
		}
		askingTwelve = job
	}
	if _, err := service.hub.DecideJob(ctx, owner, askingTwelve.ID, store.JobDecisionLater, ""); err != nil {
		t.Fatal(err)
	}
	status, body := send(t, http.MethodGet, service.url+"/v1/jobs/"+askingTwelve.ID.String(), ownerToken, "")
	var details struct {
		Fit jobfit.Fit `json:"fit"`
	}
	if err := json.Unmarshal(body, &details); status != http.StatusOK || err != nil || details.Fit.Level != jobfit.LevelGood {
		t.Fatalf("the job asking for twelve years: %d %s; want it to pass the fit", status, body)
	}

	status, body = send(t, http.MethodGet, service.url+"/v1/decision-signals", ownerToken, "")
	var signals struct {
		GoodOrUnclearSeen    int `json:"good_or_unclear_seen"`
		GoodOrUnclearDecided int `json:"good_or_unclear_decided"`
	}
	if err := json.Unmarshal(body, &signals); status != http.StatusOK || err != nil {
		t.Fatalf("signals: %d %s", status, body)
	}
	if signals.GoodOrUnclearSeen != 1 || signals.GoodOrUnclearDecided != 0 {
		t.Errorf("seen %d, decided %d; want 1 and 0, without the job asking for twelve years", signals.GoodOrUnclearSeen, signals.GoodOrUnclearDecided)
	}
}
