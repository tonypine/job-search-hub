package cvscreens

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

// fakeRecruiter answers every screen the same way and keeps what it read.
type fakeRecruiter struct {
	inputs []string
}

func (recruiter *fakeRecruiter) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	recruiter.inputs = append(recruiter.inputs, request.User)
	return chatcompletions.Answer{Model: "local-27b", Object: json.RawMessage(
		`{"issues":[{"issue":"No Go","posting_says":"Go required","response":"Name the Go service"}],"summary":"Close.","verdict":"borderline"}`,
	)}, nil
}

func TestAPursuedJobsCVIsScreenedOnceAndAgainAfterAnEdit(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	cv := resume.Resume{Basics: resume.Basics{Name: "Ada", Label: "Front-End Engineer", Summary: "Builds web apps."}}
	pursued, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Front-End Engineer", URL: "https://acme.com/1", Description: "React and Go required."})
	skipped, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{Title: "Designer", URL: "https://acme.com/2", Description: "Figma."})
	if _, err := hub.DecideJob(ctx, owner, pursued.ID, store.JobDecisionPursue, ""); err != nil {
		t.Fatal(err)
	}
	for _, job := range []store.Job{pursued, skipped} {
		if _, err := hub.SaveTailoredCV(ctx, owner, job.ID, cv, map[string]string{}); err != nil {
			t.Fatal(err)
		}
	}
	recruiter := &fakeRecruiter{}
	screener := NewScreener(hub, recruiter)

	if summary, err := screener.ScreenOnce(ctx); err != nil || summary.Screened != 1 {
		t.Fatalf("first pass = %+v, %v; want only the pursued job's CV", summary, err)
	}
	if !strings.Contains(recruiter.inputs[0], "React and Go required.") || !strings.Contains(recruiter.inputs[0], "Builds web apps.") {
		t.Errorf("the recruiter read:\n%s", recruiter.inputs[0])
	}
	screen, err := hub.GetCVScreen(ctx, pursued.ID)
	if err != nil || screen.Model != "local-27b" || !strings.Contains(string(screen.Screen), "borderline") {
		t.Fatalf("screen = %+v, %v", screen, err)
	}
	if summary, _ := screener.ScreenOnce(ctx); summary.Screened != 0 {
		t.Errorf("a screened CV was screened again: %+v", summary)
	}

	time.Sleep(10 * time.Millisecond)
	cv.Basics.Summary = "Builds web apps in React and Go."
	if _, err := hub.SaveTailoredCV(ctx, owner, pursued.ID, cv, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if summary, _ := screener.ScreenOnce(ctx); summary.Screened != 1 || !strings.Contains(recruiter.inputs[1], "React and Go.") {
		t.Errorf("after an edit: %+v", summary)
	}
}
