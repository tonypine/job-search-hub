package jobfacts_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// fakeModel answers every job with its title as the summary, except the ones
// it is told to fail on, or the doubtful answer for the doubtful title. A
// second reading is unrouted unless secondReading is set.
type fakeModel struct {
	failOn         string
	unreachable    bool
	doubtfulTitle  string
	secondReading  string
	secondReadFail bool
	requests       []chatcompletions.JSONRequest
	priorities     []modelqueue.Priority
}

const doubtfulFacts = `{"location":{"evidence":"Americas","restriction":"Americas","open_to_brazil":"unclear","reason":"x"}}`

func (model *fakeModel) CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	model.requests = append(model.requests, request)
	model.priorities = append(model.priorities, modelqueue.GetPriority(ctx, modelqueue.PriorityBackground))
	if model.unreachable {
		return chatcompletions.Answer{}, fmt.Errorf("%w: connection refused", chatcompletions.ErrUnreachable)
	}
	if request.SchemaName == store.TaskKindJobFactsSecondReading {
		switch {
		case model.secondReading == "":
			return chatcompletions.Answer{}, fmt.Errorf("no model is routed for %s: %w", request.SchemaName, store.ErrTaskRouteNotFound)
		case model.secondReadFail:
			return chatcompletions.Answer{}, fmt.Errorf("%w: connection refused", chatcompletions.ErrUnreachable)
		}
		return chatcompletions.Answer{Object: json.RawMessage(model.secondReading), Model: "second-model"}, nil
	}
	title := strings.TrimPrefix(strings.SplitN(request.User, "\n", 2)[0], "Title: ")
	if title == model.failOn {
		return chatcompletions.Answer{}, errors.New("the answer is not a JSON object")
	}
	if title == model.doubtfulTitle {
		return chatcompletions.Answer{Object: json.RawMessage(doubtfulFacts), Model: "routed-model"}, nil
	}
	return chatcompletions.Answer{Object: json.RawMessage(fmt.Sprintf(`{"summary":%q}`, title)), Model: "routed-model"}, nil
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
	extractor := jobfacts.NewExtractor(hub, model)

	summary, err := extractor.ExtractOnce(context.Background())
	if err != nil || summary.Read != 2 || summary.Failed != 1 {
		t.Fatalf("summary = %+v, err = %v", summary, err)
	}
	prompt, _ := hub.GetLatestAgentPrompt(context.Background(), store.AgentPromptKindJobFacts)
	request := model.requests[0]
	if request.System != prompt.Body || string(request.Schema) != string(prompt.ResultSchema) || request.SchemaName != store.AgentPromptKindJobFacts ||
		!strings.Contains(request.User, "Other locations: EMEA") || !strings.Contains(request.User, "Description:\nBuild things.") {
		t.Fatalf("request = %+v", request)
	}

	jobs, _, _ := hub.ListJobs(context.Background(), store.JobFilter{})
	for _, job := range jobs {
		details, err := hub.GetJobDetails(context.Background(), job.Job.ID)
		if err != nil || (details.Facts != nil && details.Facts.Model != "routed-model") {
			t.Fatalf("%s: facts = %+v, %v; want them saved with the model that answered", job.Job.Title, details.Facts, err)
		}
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

	summary, err := jobfacts.NewExtractor(hub, model).ExtractOnce(context.Background())
	if !errors.Is(err, chatcompletions.ErrUnreachable) || summary.Read != 0 || len(model.requests) != 1 {
		t.Fatalf("summary = %+v, err = %v, %d requests; want one attempt and the error", summary, err, len(model.requests))
	}
}

func TestANewPromptVersionMakesThePassReadEveryJobAgain(t *testing.T) {
	hub := startJobs(t, "Engineer", "Designer")
	model := &fakeModel{}
	extractor := jobfacts.NewExtractor(hub, model)
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

func TestTheExtractorSendsThePromptsWorkedExamples(t *testing.T) {
	hub := startJobs(t, "Engineer")
	ctx := context.Background()
	seeded, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	_, err := hub.SaveAgentPrompt(ctx, owner, store.NewAgentPrompt{Kind: store.AgentPromptKindJobFacts, Body: seeded.Body,
		ResultSchema: json.RawMessage(`{"type":"object","properties":{"summary":{"title":"Summary","description":"The role.","type":"string"}}}`),
		Examples:     []chatcompletions.Example{{Input: "Title: Example", Answer: json.RawMessage(`{"summary":"An example"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	model := &fakeModel{}
	if _, err := jobfacts.NewExtractor(hub, model).ExtractOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || len(model.requests[0].Examples) != 1 || model.requests[0].Examples[0].Input != "Title: Example" {
		t.Fatalf("requests = %+v", model.requests)
	}
}

func TestAJobReadNowRunsAsADispatchedRunEvenWithCurrentFacts(t *testing.T) {
	hub := startJobs(t, "Engineer", "Designer")
	ctx := context.Background()
	model := &fakeModel{}
	extractor := jobfacts.NewExtractor(hub, model)
	if _, err := extractor.ExtractOnce(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	model.requests, model.priorities = nil, nil
	if err := extractor.ReadJobNow(ctx, jobs[0].Job.ID); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || model.priorities[0] != modelqueue.PriorityDispatched || *model.requests[0].Task.SubjectID != jobs[0].Job.ID {
		t.Fatalf("requests = %d, priorities = %v", len(model.requests), model.priorities)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	if waiting, _ := hub.CountJobsAwaitingFacts(ctx, prompt.ID); waiting != 0 {
		t.Fatalf("%d jobs still wait for facts", waiting)
	}
	if err := extractor.ReadJobNow(ctx, uuid.New()); !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("an unknown job: %v", err)
	}
}

func TestADoubtfulReadingIsReadAgainAndMarkedWithItsDoubt(t *testing.T) {
	hub := startJobs(t, "Engineer", "Designer")
	ctx := context.Background()
	second := `{"location":{"evidence":"Americas","restriction":"Americas","open_to_brazil":"yes","reason":"Brazil is in the Americas."}}`
	model := &fakeModel{doubtfulTitle: "Engineer", secondReading: second}

	if summary, err := jobfacts.NewExtractor(hub, model).ExtractOnce(ctx); err != nil || summary.Read != 2 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, err = %v", summary, err)
	}
	var secondRequests []chatcompletions.JSONRequest
	for _, request := range model.requests {
		if request.SchemaName == store.TaskKindJobFactsSecondReading {
			secondRequests = append(secondRequests, request)
		}
	}
	if len(model.requests) != 3 || len(secondRequests) != 1 || !strings.HasPrefix(secondRequests[0].User, "Title: Engineer") ||
		secondRequests[0].System != model.requests[0].System || string(secondRequests[0].Schema) != string(model.requests[0].Schema) {
		t.Fatalf("requests = %+v; want one second reading of the doubtful job, with the same prompt", model.requests)
	}

	jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	for _, job := range jobs {
		details, err := hub.GetJobDetails(ctx, job.Job.ID)
		if err != nil || details.Facts == nil {
			t.Fatalf("%s: %+v, %v", job.Job.Title, details.Facts, err)
		}
		facts := details.Facts
		switch job.Job.Title {
		case "Engineer":
			if facts.Model != "second-model" || facts.FirstModel != "routed-model" || facts.Doubt != jobfacts.DoubtOpenToBrazilUnclear ||
				!strings.Contains(string(details.RawFacts), "Brazil is in the Americas.") {
				t.Errorf("doubtful job's facts = %+v %s; want the second reading, marked", facts, details.RawFacts)
			}
		default:
			if facts.Model != "routed-model" || facts.Doubt != "" || facts.FirstModel != "" {
				t.Errorf("clear job's facts = %+v; want the first reading, unmarked", facts)
			}
		}
	}
}

func TestADoubtfulReadingIsKeptWhenNoSecondReadingCanBeHad(t *testing.T) {
	for _, model := range []*fakeModel{
		{doubtfulTitle: "Engineer"},
		{doubtfulTitle: "Engineer", secondReading: `{}`, secondReadFail: true},
	} {
		hub := startJobs(t, "Engineer")
		ctx := context.Background()
		if summary, err := jobfacts.NewExtractor(hub, model).ExtractOnce(ctx); err != nil || summary.Read != 1 || len(model.requests) != 2 {
			t.Fatalf("summary = %+v, err = %v, %d requests; want the job read, and a second reading tried", summary, err, len(model.requests))
		}
		jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
		details, err := hub.GetJobDetails(ctx, jobs[0].Job.ID)
		if err != nil || details.Facts == nil || details.Facts.Model != "routed-model" || details.Facts.Doubt != "" || !strings.Contains(string(details.RawFacts), "unclear") {
			t.Fatalf("facts = %+v %s, %v; want the first reading kept, unmarked", details.Facts, details.RawFacts, err)
		}
	}
}
