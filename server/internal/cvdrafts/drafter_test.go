package cvdrafts

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var base = resume.Resume{
	Basics: resume.Basics{Name: "Ada", Label: "Engineer", Summary: "Builds."},
	Work: []resume.Work{
		{Name: "Acme", Position: "Senior Engineer", StartDate: "2022-01", Highlights: []string{"Built the API.", "Led the web app."}},
		{Name: "Initech", Position: "Engineer", StartDate: "2019-01", EndDate: "2021-12", Highlights: []string{"Wrote reports."}},
	},
}

func TestACitationMustBeABaseBulletOrAConfirmedEntry(t *testing.T) {
	confirmed := store.ProfileEntry{ID: uuid.New()}
	for _, test := range []struct {
		name   string
		source string
		role   int
		ok     bool
	}{
		{"a base bullet", "base:w0h1", 0, true},
		{"a confirmed entry", "entry:" + confirmed.ID.String(), 0, true},
		{"no source", "", 0, false},
		{"an unknown base bullet", "base:w0h9", 0, false},
		{"an unconfirmed entry", "entry:" + uuid.NewString(), 0, false},
		{"a role the base CV doesn't have", "base:w0h0", 5, false},
	} {
		err := checkCitations(Draft{Roles: []DraftRole{{Role: test.role, Bullets: []DraftBullet{{Text: "x", Source: test.source}}}}}, base, []store.ProfileEntry{confirmed})
		if (err == nil) != test.ok || (err != nil && !errors.Is(err, chatcompletions.ErrInvalidAnswer)) {
			t.Errorf("%s: error = %v", test.name, err)
		}
	}
}

func TestATailoredCVKeepsTheBaseFactsAndTakesTheDraftsWords(t *testing.T) {
	tailored, citations := assembleTailoredCV(base, Draft{
		Label: "Front-End Engineer", Summary: "Builds web apps.",
		Roles: []DraftRole{{Role: 0, Bullets: []DraftBullet{{Text: "Led the React web app.", Source: "base:w0h1"}}}},
	})
	if tailored.Basics.Label != "Front-End Engineer" || tailored.Basics.Summary != "Builds web apps." || tailored.Basics.Name != "Ada" {
		t.Errorf("basics = %+v", tailored.Basics)
	}
	if len(tailored.Work) != 2 || tailored.Work[0].StartDate != "2022-01" || tailored.Work[0].Highlights[0] != "Led the React web app." || len(tailored.Work[0].Highlights) != 1 {
		t.Errorf("work = %+v", tailored.Work)
	}
	if tailored.Work[1].Highlights[0] != "Wrote reports." || base.Work[0].Highlights[0] != "Built the API." {
		t.Error("a role the draft left out lost its bullets, or the base CV changed")
	}
	if citations["w0h0"] != "base:w0h1" || citations["w0h1"] != "" || citations["w1h0"] != "base:w1h0" {
		t.Errorf("citations = %v", citations)
	}
}

// fakeClaude answers with answer, and keeps the request it was sent.
type fakeClaude struct {
	answer  string
	request chatcompletions.JSONRequest
}

func (claude *fakeClaude) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	claude.request = request
	return chatcompletions.Answer{Model: "claude-sonnet", Object: json.RawMessage(claude.answer)}, nil
}

func TestAPursuedJobGetsADraftAndABadCitationSavesNothing(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := hub.SaveBaseCV(ctx, owner, base); err != nil {
		t.Fatal(err)
	}
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Front-End Engineer", URL: "https://acme.com/1", Description: "React."})
	if _, err := hub.DecideJob(ctx, owner, job.ID, store.JobDecisionPursue, ""); err != nil {
		t.Fatal(err)
	}
	if awaiting, _ := hub.ListJobsAwaitingCV(ctx); len(awaiting) != 1 || awaiting[0] != job.ID {
		t.Fatalf("awaiting = %v", awaiting)
	}

	bad := &fakeClaude{answer: `{"label":"x","summary":"y","roles":[{"role":0,"bullets":[{"text":"Invented.","source":"entry:` + uuid.NewString() + `"}]}]}`}
	if _, err := NewDrafter(hub, bad).DraftCV(ctx, job.ID); !errors.Is(err, chatcompletions.ErrInvalidAnswer) {
		t.Fatalf("a bad citation: %v", err)
	}
	if _, err := hub.GetJobCV(ctx, job.ID); !errors.Is(err, store.ErrCVNotFound) {
		t.Fatalf("a refused draft was saved: %v", err)
	}

	good := &fakeClaude{answer: `{"label":"Front-End Engineer","summary":"Builds web apps.","roles":[{"role":0,"bullets":[{"text":"Led the web app.","source":"base:w0h1"}]}]}`}
	cv, err := NewDrafter(hub, good).DraftCV(ctx, job.ID)
	if err != nil || cv.Kind != store.CVKindTailored || cv.Citations["w0h0"] != "base:w0h1" || cv.Content.Basics.Label != "Front-End Engineer" {
		t.Fatalf("cv = %+v, %v", cv, err)
	}
	if !strings.Contains(good.request.User, "[base:w0h1] Led the web app.") || !strings.Contains(good.request.User, "None are confirmed yet") {
		t.Errorf("input =\n%s", good.request.User)
	}
	if awaiting, _ := hub.ListJobsAwaitingCV(ctx); len(awaiting) != 0 {
		t.Errorf("still awaiting a CV: %v", awaiting)
	}
	if details, _ := hub.GetJobDetails(ctx, job.ID); details.CVID == nil || *details.CVID != cv.ID {
		t.Errorf("details cv_id = %v", details.CVID)
	}
	if _, err := NewDrafter(hub, nil).DraftCV(ctx, job.ID); !errors.Is(err, ErrNoCVDrafts) {
		t.Errorf("without Claude: %v", err)
	}
}

func TestAnEditKeepsItsCitationsAndAPrintedPDFIsKept(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := hub.SaveBaseCV(ctx, owner, base); err != nil {
		t.Fatal(err)
	}
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/1"})
	drafter := NewDrafter(hub, &fakeClaude{answer: `{"label":"L","summary":"S","roles":[]}`})
	if _, err := drafter.SaveEdit(ctx, owner, job.ID, Draft{}); !errors.Is(err, store.ErrCVNotFound) {
		t.Fatalf("editing a job without a draft: %v", err)
	}
	cv, err := drafter.DraftCV(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := hub.SaveCVPDF(ctx, owner, cv.ID, []byte("%PDF-1.4 x"), "/tmp/cv.pdf"); err != nil {
		t.Fatal(err)
	}
	if printed, _ := hub.GetCV(ctx, cv.ID); !printed.HasPDF {
		t.Error("the PDF wasn't kept")
	}

	edited, err := drafter.SaveEdit(ctx, owner, job.ID, Draft{Label: "Mine", Summary: "My words.",
		Roles: []DraftRole{{Role: 0, Bullets: []DraftBullet{{Text: "Led the web app, my way.", Source: "base:w0h1"}}}}})
	if err != nil || edited.Content.Basics.Label != "Mine" || edited.Citations["w0h0"] != "base:w0h1" || edited.HasPDF {
		t.Fatalf("edited = %+v, %v; want the edit saved with its citation and the stale PDF dropped", edited, err)
	}
	if _, err := drafter.SaveEdit(ctx, owner, job.ID, Draft{Roles: []DraftRole{{Role: 0, Bullets: []DraftBullet{{Text: "Mine.", Source: ""}}}}}); !errors.Is(err, chatcompletions.ErrInvalidAnswer) {
		t.Errorf("an edit dropping a citation: %v", err)
	}
}

var owner = store.Actor{Kind: store.ActorOwner}

// fakePrinter records what it was asked to print and marks it printed.
type fakePrinter struct {
	hub     *store.Store
	printed []uuid.UUID
}

func (printer *fakePrinter) PrintCV(ctx context.Context, actor store.Actor, cvID uuid.UUID) (store.CV, error) {
	printer.printed = append(printer.printed, cvID)
	if err := printer.hub.SaveCVPDF(ctx, actor, cvID, []byte("%PDF-fake"), "/cvs/"+cvID.String()+".pdf"); err != nil {
		return store.CV{}, err
	}
	return printer.hub.GetCV(ctx, cvID)
}

func addJobWithFacts(t *testing.T, hub *store.Store, title, location, facts string) store.Job {
	t.Helper()
	ctx := context.Background()
	job, _, err := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: title, URL: "https://acme.com/" + title, Location: location, Description: "Build."})
	if err != nil {
		t.Fatal(err)
	}
	prompt, _ := hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindJobFacts)
	awaiting, _ := hub.GetJobForFacts(ctx, job.ID)
	if err := hub.SaveJobFacts(ctx, store.NewJobFacts{JobID: job.ID, PromptID: prompt.ID, Model: "m", TextHash: awaiting.TextHash, Facts: json.RawMessage(facts)}); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestGoodFitsAndPursuedJobsGetAPrintedCVOnce(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveBaseCV(ctx, owner, base); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SaveJobCriteria(ctx, owner, store.JobCriteria{Roles: []string{"Product Engineer"}, SeniorityLevels: []string{"Senior"},
		Technologies: []string{"React"}, HomeCountry: "Brazil", EligibleLocationTerms: []string{"LATAM"}}); err != nil {
		t.Fatal(err)
	}
	good := addJobWithFacts(t, hub, "Senior Product Engineer", "LATAM", `{"technologies":["React"],"location_restriction":"LATAM","seniority":"Senior"}`)
	poor := addJobWithFacts(t, hub, "Sales Manager", "US only", `{"technologies":["Salesforce"],"location_restriction":"US only"}`)
	pursued := addJobWithFacts(t, hub, "Platform Engineer", "", `{"technologies":["Go"]}`)
	if _, err := hub.DecideJob(ctx, owner, pursued.ID, store.JobDecisionPursue, ""); err != nil {
		t.Fatal(err)
	}
	claude := &fakeClaude{answer: `{"label":"Product Engineer","summary":"Builds.","roles":[{"role":0,"bullets":[{"text":"Led the web app.","source":"base:w0h1"}]}]}`}
	drafter := NewDrafter(hub, claude)
	printer := &fakePrinter{hub: hub}
	drafter.Printer = printer

	needing, err := drafter.ListJobsNeedingCV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(needing) != 2 || needing[0] != pursued.ID || needing[1] != good.ID || slices.Contains(needing, poor.ID) {
		t.Fatalf("needing = %v, want the pursued job, then the good fit, and not the poor one", needing)
	}

	cv, err := drafter.GenerateCV(ctx, good.ID)
	if err != nil || !cv.HasPDF || cv.PDFPath == "" {
		t.Fatalf("generated = %+v, %v", cv, err)
	}
	again, err := drafter.GenerateCV(ctx, good.ID)
	if err != nil || again.ID != cv.ID || len(printer.printed) != 1 {
		t.Errorf("a second generate: %+v, %v; printed %d times, want once and no new draft", again, err, len(printer.printed))
	}
	if needing, _ := drafter.ListJobsNeedingCV(ctx); len(needing) != 1 || needing[0] != pursued.ID {
		t.Errorf("after generating, needing = %v, want only the pursued job", needing)
	}
}
