package mailtriage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestRulesSortTheObviousMailAndLeaveTheRestToTheModel(t *testing.T) {
	companyID := uuid.New()
	received := func(sender, subject string, labels ...string) store.MailMessage {
		return store.MailMessage{Direction: store.MailReceived, Sender: sender, Subject: subject, LabelIDs: labels}
	}
	aboutACompany := received("Ada <ada@acme.com>", "Re: Engineer")
	aboutACompany.CompanyID = &companyID

	cases := []struct {
		name      string
		message   store.MailMessage
		wantClass string
	}{
		{"an Indeed alert", received("Indeed <donotreply@match.indeed.com>", "Front-end Pleno na Acme"), store.MailJobAlert},
		{"a LinkedIn job alert", received("LinkedIn <jobs-noreply@linkedin.com>", "Senior Frontend at Acme"), store.MailJobAlert},
		{"a Glassdoor alert", received("Vagas do Glassdoor <noreply@glassdoor.com>", "As melhores vagas"), store.MailJobAlert},
		{"an application through Indeed", received("Indeed <indeedapply@indeed.com>", "Inscrição via Indeed: Engineer"), store.MailApplicationConfirmation},
		{"a confirmation from a no-reply address", received("Acme No Reply <no-reply@acme.com>", "We’ve Received Your Application – Acme"), store.MailApplicationConfirmation},
		{"a Portuguese confirmation", received("vagas@naoresponda.acme.com.br", "Recebemos sua candidatura!"), store.MailApplicationConfirmation},
		{"a person writing an application-received subject", received("Ada <ada@acme.com>", "Application received, next steps", "CATEGORY_PERSONAL"), ""},
		{"mail about a company the hub knows", aboutACompany, ""},
		{"mail in the Personal category", received("Pat <pat@gmail.com>", "Quick question", "CATEGORY_PERSONAL"), ""},
		{"a LinkedIn message", received("LinkedIn <messaging-digest-noreply@linkedin.com>", "Geovanne just messaged you"), ""},
		{"a newsletter", received("News <news@example.com>", "This week", "CATEGORY_UPDATES"), store.MailNoise},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			classification, found := ClassifyByRule(testCase.message)
			if testCase.wantClass == "" {
				if found {
					t.Fatalf("classified %+v; want it left to the model", classification)
				}
				return
			}
			if !found || classification.Class != testCase.wantClass || classification.ClassifiedBy != store.ClassifiedByRule {
				t.Fatalf("classification = %+v, %v; want %s", classification, found, testCase.wantClass)
			}
		})
	}
}

type fakeModel struct {
	answers     map[string]string
	unreachable bool
	requests    []chatcompletions.JSONRequest
}

func (model *fakeModel) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (json.RawMessage, error) {
	model.requests = append(model.requests, request)
	if model.unreachable {
		return nil, fmt.Errorf("%w: connection refused", chatcompletions.ErrUnreachable)
	}
	for subject, answer := range model.answers {
		if strings.Contains(request.User, "Subject: "+subject) {
			return json.RawMessage(answer), nil
		}
	}
	return nil, errors.New("no answer for this message")
}

type fakeMailbox struct{}

func (fakeMailbox) GetMessage(_ context.Context, id string) (google.Message, error) {
	if id == "gone" {
		return google.Message{}, google.ErrMessageNotFound
	}
	return google.Message{Text: "The text of " + id}, nil
}

func TestAPassSortsByRuleAndModelAndRetriesWhatTheModelCouldNotRead(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	acme, _, _ := hub.CreateCompany(ctx, store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	record := func(id, sender, subject string, labels ...string) store.MailMessage {
		message, _, err := hub.RecordMailMessage(ctx, store.NewMailMessage{
			GmailMessageID: id, ThreadID: id, Direction: store.MailReceived, Sender: sender, Subject: subject, SentAt: time.Now(), LabelIDs: labels,
		})
		if err != nil {
			t.Fatal(err)
		}
		return message
	}
	invite := record("invite", "Ada <ada@acme.com>", "Next steps")
	if err := hub.SaveMailMatch(ctx, invite.ID, store.MailMatch{CompanyID: acme.ID, MatchedBy: store.MatchedByDomain}); err != nil {
		t.Fatal(err)
	}
	record("alert", "donotreply@match.indeed.com", "Vagas")
	record("personal", "Pat <pat@gmail.com>", "Hello", "CATEGORY_PERSONAL")
	record("gone", "Kim <kim@gmail.com>", "Deleted", "CATEGORY_PERSONAL")
	model := &fakeModel{unreachable: true, answers: map[string]string{
		"Next steps": `{"class":"interview_invite","reason":"asks to book a call"}`,
		"Hello":      `{"class":"noise","reason":"a friend"}`,
	}}
	classifier := NewClassifier(hub, fakeMailbox{}, model, "test-model")

	summary, err := classifier.ClassifyOnce(ctx)
	if !errors.Is(err, chatcompletions.ErrUnreachable) || summary.ByRule != 2 || summary.ByModel != 0 {
		t.Fatalf("with the model down: %+v, %v; want the alert and the deleted message sorted by rule, then the pass stopped", summary, err)
	}

	model.unreachable = false
	summary, err = classifier.ClassifyOnce(ctx)
	if err != nil || summary.ByModel != 2 || summary.Failed != 0 {
		t.Fatalf("second pass = %+v, %v", summary, err)
	}
	classes := map[string]store.MailMessage{}
	all, _ := hub.ListMailMessages(ctx, store.MailFilter{Limit: 10})
	for _, message := range all {
		classes[message.GmailMessageID] = message
	}
	if got := classes["invite"]; got.Classification != store.MailInterviewInvite || got.ClassifiedBy != store.ClassifiedByModel || got.ClassificationReason != "asks to book a call" {
		t.Fatalf("invite = %+v", got)
	}
	if classes["alert"].Classification != store.MailJobAlert || classes["personal"].Classification != store.MailNoise || classes["gone"].Classification != store.MailNoise {
		t.Fatalf("classes = %+v", classes)
	}
	var readTheCompany, readTheText bool
	for _, request := range model.requests {
		readTheCompany = readTheCompany || strings.Contains(request.User, "Matched company: Acme")
		readTheText = readTheText || strings.Contains(request.User, "Text:\nThe text of invite")
	}
	if !readTheCompany || !readTheText {
		t.Fatalf("the model should read the matched company and the text: %+v", model.requests)
	}

	if err := hub.SaveMailMatch(ctx, classes["personal"].ID, store.MailMatch{CompanyID: acme.ID, MatchedBy: store.MatchedByThread}); err != nil {
		t.Fatal(err)
	}
	if awaiting, _ := hub.ListMailAwaitingClassification(ctx, 10); len(awaiting) != 1 || awaiting[0].GmailMessageID != "personal" {
		t.Fatalf("awaiting = %+v; a message matched anew is read again", awaiting)
	}
}
