package conversationtriage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

type fakeModel struct {
	unreachable bool
	requests    []string
}

func (model *fakeModel) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (json.RawMessage, error) {
	model.requests = append(model.requests, request.User)
	if model.unreachable {
		return nil, fmt.Errorf("%w: connection refused", chatcompletions.ErrUnreachable)
	}
	if strings.Contains(request.User, "a role at Globex") {
		return json.RawMessage(`{"class":"recruiter_outreach","company":"Globex","role":"Senior Frontend Engineer","is_agency":false,"reason":"offers a role"}`), nil
	}
	return json.RawMessage(`{"class":"known_person","company":"","role":"known_person","is_agency":false,"reason":"a friend"}`), nil
}

func TestConversationsOthersStartedAreSortedAndRecruitersNamed(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	owner := "https://www.linkedin.com/in/owner-example"
	at := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	message := func(conversation, sender, content string, minutes int) store.NewLinkedInMessage {
		recipient := owner
		if sender == owner {
			recipient = "https://www.linkedin.com/in/someone"
		}
		return store.NewLinkedInMessage{ConversationID: conversation, SenderName: "Someone", SenderProfileURL: sender,
			RecipientProfileURLs: []string{recipient}, SentAt: at.Add(time.Duration(minutes) * time.Minute), Content: content}
	}
	if _, err := hub.ImportLinkedInMessages(ctx, store.Actor{Kind: store.ActorOwner}, []store.NewLinkedInMessage{
		message("recruiter", "https://www.linkedin.com/in/rita-example", "Hi, a role at Globex for you", 0),
		message("friend", "https://www.linkedin.com/in/ada-example", "Lunch?", 1),
		message("empty", "https://www.linkedin.com/in/grace-example", "", 2),
		message("mine", owner, "Hello, I saw your post", 3),
		message("mine", owner, "Following up", 4),
	}); err != nil {
		t.Fatal(err)
	}
	model := &fakeModel{unreachable: true}
	classifier := NewClassifier(hub, model, "test-model")

	if _, err := classifier.ClassifyOnce(ctx); !errors.Is(err, chatcompletions.ErrUnreachable) {
		t.Fatalf("with the model down: err = %v", err)
	}
	model.unreachable = false
	summary, err := classifier.ClassifyOnce(ctx)
	if err != nil || summary.ByModel+summary.ByRule < 2 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	if again, _ := classifier.ClassifyOnce(ctx); again != (PassSummary{}) {
		t.Fatalf("a second pass = %+v; every conversation others started is classified once", again)
	}
	for _, request := range model.requests {
		if strings.Contains(request, "I saw your post") {
			t.Fatal("a conversation the owner started should not be classified")
		}
	}

	recruiters, err := hub.ListRecruiterConversations(ctx)
	if err != nil || len(recruiters) != 1 || recruiters[0].HiringCompany != "Globex" || recruiters[0].Role != "Senior Frontend Engineer" || recruiters[0].OwnerWrote {
		t.Fatalf("recruiters = %+v, %v", recruiters, err)
	}
}
