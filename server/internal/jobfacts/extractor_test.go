package jobfacts_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// fakeModel answers every job with its title as the summary, except the ones
// it is told to fail on.
type fakeModel struct {
	failOn      string
	unreachable bool
	requests    []chatcompletions.JSONRequest
}

func (model *fakeModel) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (json.RawMessage, error) {
	model.requests = append(model.requests, request)
	if model.unreachable {
		return nil, fmt.Errorf("%w: connection refused", chatcompletions.ErrUnreachable)
	}
	title := strings.TrimPrefix(strings.SplitN(request.User, "\n", 2)[0], "Title: ")
	if title == model.failOn {
		return nil, errors.New("the answer is not a JSON object")
	}
	return json.RawMessage(fmt.Sprintf(`{"summary":%q}`, title)), nil
}

func startJobs(t *testing.T, titles ...string) *store.Store {
	t.Helper()
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	board, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: company.ID, Provider: "lever", BoardToken: "acme", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	var postings []store.JobPosting
	for index, title := range titles {
		postings = append(postings, store.JobPosting{
			ExternalID: fmt.Sprint(index), Title: title, Location: "Remote", URL: fmt.Sprintf("https://jobs.lever.co/acme/%d", index),
			Description: "Build things.", BoardFacts: store.BoardFacts{OtherLocations: []string{"EMEA"}},
		})
	}
	if _, err := hub.SyncBoardJobs(ctx, store.Actor{Kind: store.ActorSystem}, board, postings, time.Now()); err != nil {
		t.Fatal(err)
	}
	return hub
}

func TestAPassReadsEveryJobAndSkipsTheOneTheModelFailsOn(t *testing.T) {
	hub := startJobs(t, "Engineer", "Designer", "Manager")
	model := &fakeModel{failOn: "Designer"}
	extractor := jobfacts.NewExtractor(hub, model, "test-model")

	summary, err := extractor.ExtractOnce(context.Background())
	if err != nil || summary.Read != 2 || summary.Failed != 1 {
		t.Fatalf("summary = %+v, err = %v", summary, err)
	}
	prompt, _ := hub.GetLatestAgentPrompt(context.Background(), store.AgentPromptKindJobFacts)
	request := model.requests[0]
	if request.System != prompt.Body || string(request.Schema) != string(prompt.ResultSchema) || request.Model != "test-model" ||
		!strings.Contains(request.User, "Other locations: EMEA") || !strings.Contains(request.User, "Description:\nBuild things.") {
		t.Fatalf("request = %+v", request)
	}

	// The next pass only retries the failed job.
	model.failOn = ""
	model.requests = nil
	if summary, err := extractor.ExtractOnce(context.Background()); err != nil || summary.Read != 1 || len(model.requests) != 1 {
		t.Fatalf("second pass = %+v, %v, %d requests", summary, err, len(model.requests))
	}
}

func TestAnUnreachableModelStopsThePass(t *testing.T) {
	hub := startJobs(t, "Engineer", "Designer")
	model := &fakeModel{unreachable: true}

	summary, err := jobfacts.NewExtractor(hub, model, "test-model").ExtractOnce(context.Background())
	if !errors.Is(err, chatcompletions.ErrUnreachable) || summary.Read != 0 || len(model.requests) != 1 {
		t.Fatalf("summary = %+v, err = %v, %d requests; want one attempt and the error", summary, err, len(model.requests))
	}
}

func TestANewPromptVersionMakesThePassReadEveryJobAgain(t *testing.T) {
	hub := startJobs(t, "Engineer", "Designer")
	model := &fakeModel{}
	extractor := jobfacts.NewExtractor(hub, model, "test-model")
	if _, err := extractor.ExtractOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveAgentPrompt(context.Background(), owner, store.NewAgentPrompt{Kind: store.AgentPromptKindJobFacts, Body: "Record more."}); err != nil {
		t.Fatal(err)
	}

	if summary, err := extractor.ExtractOnce(context.Background()); err != nil || summary.Read != 2 {
		t.Fatalf("summary after a new prompt = %+v, %v", summary, err)
	}
}
