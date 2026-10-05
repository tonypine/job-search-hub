package mailactions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

type recordedUpdates struct {
	hub     *store.Store
	updates []store.NewUpdate
}

func (recorder *recordedUpdates) Record(ctx context.Context, input store.NewUpdate) (store.Update, error) {
	recorder.updates = append(recorder.updates, input)
	return recorder.hub.RecordUpdate(ctx, input)
}

type fixture struct {
	t       *testing.T
	ctx     context.Context
	hub     *store.Store
	handler *Handler
	updates *recordedUpdates
	phases  map[string]store.PipelinePhase
	acme    store.Company
}

func startFixture(t *testing.T) *fixture {
	t.Helper()
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	if _, err := hub.SaveGoogleConnection(ctx, owner, "owner@example.com", "refresh-1", google.Scopes); err != nil {
		t.Fatal(err)
	}
	acme, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	phases := map[string]store.PipelinePhase{}
	listed, _ := hub.ListPipelinePhases(ctx)
	for _, phase := range listed {
		phases[phase.Name] = phase
	}
	updates := &recordedUpdates{hub: hub}
	return &fixture{t: t, ctx: ctx, hub: hub, handler: NewHandler(hub, updates), updates: updates, phases: phases, acme: acme}
}

// receive records a classified message, about Acme when companyID is set.
func (f *fixture) receive(sender, subject, class string, sentAt time.Time, companyID *uuid.UUID) {
	f.t.Helper()
	f.record(store.MailReceived, sender, "owner@example.com", subject, sentAt, companyID, class)
}

func (f *fixture) record(direction, sender, recipients, subject string, sentAt time.Time, companyID *uuid.UUID, class string) {
	f.t.Helper()
	message, _, err := f.hub.RecordMailMessage(f.ctx, store.NewMailMessage{
		GmailMessageID: uuid.NewString(), ThreadID: "thread", Direction: direction, Sender: sender, Recipients: recipients,
		Subject: subject, SentAt: sentAt,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	if companyID != nil {
		if err := f.hub.SaveMailMatch(f.ctx, message.ID, store.MailMatch{CompanyID: *companyID, MatchedBy: store.MatchedByDomain}); err != nil {
			f.t.Fatal(err)
		}
	}
	if class != "" {
		if err := f.hub.SaveMailClassification(f.ctx, message.ID, store.MailClassification{Class: class, ClassifiedBy: store.ClassifiedByModel}); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *fixture) act() int {
	f.t.Helper()
	acted, err := f.handler.ActOnce(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return acted
}

func (f *fixture) card() store.Application {
	f.t.Helper()
	application, err := f.hub.FindCompanyApplication(f.ctx, f.acme.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return application
}

func (f *fixture) addCard(phaseName string) store.Application {
	f.t.Helper()
	application, _, err := f.hub.AddApplication(f.ctx, store.Actor{Kind: store.ActorOwner}, store.ApplicationInput{CompanyID: &f.acme.ID})
	if err != nil {
		f.t.Fatal(err)
	}
	if phaseName != "Saved" {
		if application, err = f.hub.MoveApplication(f.ctx, store.Actor{Kind: store.ActorOwner}, application.ID, f.phases[phaseName].ID, ""); err != nil {
			f.t.Fatal(err)
		}
	}
	return application
}

func TestAConfirmationDatesTheApplicationByTheMail(t *testing.T) {
	f := startFixture(t)
	f.addCard("Applied")
	confirmedAt := time.Now().Add(-72 * time.Hour).UTC().Truncate(time.Second)

	f.receive("Acme No Reply <no-reply@acme.com>", "We received your application", store.MailApplicationConfirmation, confirmedAt, &f.acme.ID)
	if acted := f.act(); acted != 1 {
		t.Fatalf("acted = %d", acted)
	}

	card := f.card()
	if card.PhaseID != f.phases["Applied"].ID || !card.PhaseEnteredAt.Equal(confirmedAt) || card.ContactedAt != nil {
		t.Fatalf("card = %+v; want Applied since the mail's date, and no contact from an automated note", card)
	}
	if card.AppliedAt == nil || !card.AppliedAt.Equal(confirmedAt) {
		t.Fatalf("card went out at %v; want the mail's date %v", card.AppliedAt, confirmedAt)
	}
	if len(f.updates.updates) != 1 || f.updates.updates[0].Title != "Acme confirmed your application" ||
		!strings.Contains(f.updates.updates[0].Body, "dated the application") || !strings.Contains(f.updates.updates[0].SourceURL, "authuser=owner%40example.com#all/") {
		t.Fatalf("updates = %+v", f.updates.updates)
	}
	if acted := f.act(); acted != 0 || len(f.updates.updates) != 1 {
		t.Fatalf("a second pass acted on %d messages; each message is acted on once", acted)
	}
}

func TestAConfirmationMovesASavedCardToAppliedGoneOutAtTheMailsDate(t *testing.T) {
	f := startFixture(t)
	f.addCard("Saved")
	confirmedAt := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)

	f.receive("Acme No Reply <no-reply@acme.com>", "We received your application", store.MailApplicationConfirmation, confirmedAt, &f.acme.ID)
	f.act()

	card := f.card()
	if card.PhaseID != f.phases["Applied"].ID || card.AppliedAt == nil || !card.AppliedAt.Equal(confirmedAt) {
		t.Fatalf("card = %+v; want it in Applied, gone out at %v", card, confirmedAt)
	}
}

func TestRepliesAndInvitesMoveTheCardForwardOnlyAndCountAsContact(t *testing.T) {
	f := startFixture(t)
	f.addCard("Applied")
	firstReplyAt := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)

	f.receive("Ada Lovelace <ada@acme.com>", "Re: Engineer", store.MailHumanReply, firstReplyAt, &f.acme.ID)
	f.receive("Ada Lovelace <ada@acme.com>", "Let's talk", store.MailInterviewInvite, firstReplyAt.Add(time.Hour), &f.acme.ID)
	f.receive("Ada Lovelace <ada@acme.com>", "Re: Let's talk", store.MailHumanReply, firstReplyAt.Add(2*time.Hour), &f.acme.ID)
	f.act()

	card := f.card()
	if card.PhaseID != f.phases["Interviewing"].ID || card.ContactedAt == nil || !card.ContactedAt.Equal(firstReplyAt) {
		t.Fatalf("card = %+v; want Interviewing, not moved back by the later reply, and contacted at the first reply", card)
	}
	if titles := []string{f.updates.updates[0].Title, f.updates.updates[1].Title}; titles[0] != "Ada Lovelace replied" || titles[1] != "Acme invited you to interview" {
		t.Fatalf("titles = %v", titles)
	}
	if body := f.updates.updates[0].Body; body != "Re: Engineer\nThe hub moved the card to In contact; noted the first contact." {
		t.Fatalf("body = %q", body)
	}
}

func TestAPersonsRejectionClosesTheCardAndStillCountsAsContact(t *testing.T) {
	f := startFixture(t)
	f.addCard("Applied")

	f.receive("Ada Lovelace <ada@acme.com>", "Re: Quick note", store.MailRejection, time.Now().Add(-time.Hour), &f.acme.ID)
	f.act()

	card := f.card()
	if card.PhaseID != f.phases["Closed"].ID || card.ClosedReason != "Rejected by mail: Re: Quick note" || card.ContactedAt == nil {
		t.Fatalf("card = %+v", card)
	}
}

func TestMailSentToACompanyWithAnOpenCardIsAFollowUp(t *testing.T) {
	f := startFixture(t)
	f.addCard("Applied")
	sentAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)

	f.record(store.MailSent, "owner@example.com", "Ada <ada@acme.com>", "Checking in", sentAt, &f.acme.ID, "")
	f.act()

	if card := f.card(); card.LastFollowedUpAt == nil || !card.LastFollowedUpAt.Equal(sentAt) || len(f.updates.updates) != 0 {
		t.Fatalf("card = %+v, updates = %+v; want the follow-up dated by the mail and no update", card, f.updates.updates)
	}
}

func TestRecruiterOutreachFromACompanyAddressAddsTheCompanyTheRecruiterAndACard(t *testing.T) {
	f := startFixture(t)

	f.receive("Rita Recruiter <rita@talent.globex.io>", "A role you might like", store.MailRecruiterOutreach, time.Now().Add(-time.Hour), nil)
	f.receive("Geo via LinkedIn <messaging-digest-noreply@linkedin.com>", "Geo just messaged you", store.MailRecruiterOutreach, time.Now(), nil)
	f.act()

	globex, err := f.hub.GetCompanyByDomain(f.ctx, "talent.globex.io")
	if err != nil || globex.Name != "Globex" {
		t.Fatalf("company = %+v, %v", globex, err)
	}
	people, _ := f.hub.ListPeople(f.ctx, globex.ID)
	if len(people) != 1 || people[0].Email != "rita@talent.globex.io" || people[0].Relevance != "recruiter" {
		t.Fatalf("people = %+v", people)
	}
	card, err := f.hub.FindCompanyApplication(f.ctx, globex.ID)
	if err != nil || card.PhaseID != f.phases["In contact"].ID || card.ContactedAt == nil {
		t.Fatalf("card = %+v, %v", card, err)
	}
	if len(f.updates.updates) != 2 || f.updates.updates[0].Title != "Rita Recruiter reached out" || f.updates.updates[1].Title != "Geo reached out" ||
		f.updates.updates[1].CompanyID != nil {
		t.Fatalf("updates = %+v; a relayed message is news without a company", f.updates.updates)
	}
}

func TestOldMailThatChangesNothingIsNoNews(t *testing.T) {
	f := startFixture(t)
	card := f.addCard("Interviewing")
	if _, err := f.hub.MarkApplicationContacted(f.ctx, mailWatcher, card.ID, time.Now().Add(-60*24*time.Hour), ""); err != nil {
		t.Fatal(err)
	}

	f.receive("Ada <ada@acme.com>", "Old thread", store.MailHumanReply, time.Now().Add(-30*24*time.Hour), &f.acme.ID)
	if acted := f.act(); acted != 1 || len(f.updates.updates) != 0 {
		t.Fatalf("acted = %d, updates = %+v", acted, f.updates.updates)
	}
}

func TestACompanyIsNamedAfterItsDomain(t *testing.T) {
	for domain, want := range map[string]string{"acme.com": "Acme", "talent.acme.io": "Acme", "acme.com.br": "Acme", "mail.acme.co.uk": "Acme"} {
		if got := getCompanyNameFromDomain(domain); got != want {
			t.Fatalf("%s: got %q, want %q", domain, got, want)
		}
	}
}

func TestOnlySomeoneAtTheCompanyCountsAsContact(t *testing.T) {
	f := startFixture(t)
	f.addCard("Applied")
	introducedAt := time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Second)
	message, _, _ := f.hub.RecordMailMessage(f.ctx, store.NewMailMessage{
		GmailMessageID: "intro", ThreadID: "thread", Direction: store.MailReceived, Sender: "Pat <pat@example.dev>", Subject: "Re: Quick note", SentAt: introducedAt,
	})
	f.hub.SaveMailMatch(f.ctx, message.ID, store.MailMatch{CompanyID: f.acme.ID, MatchedBy: store.MatchedByThread})
	f.hub.SaveMailClassification(f.ctx, message.ID, store.MailClassification{Class: store.MailHumanReply, ClassifiedBy: store.ClassifiedByModel})
	repliedAt := introducedAt.Add(time.Hour)
	f.receive("Ada <ada@acme.com>", "Re: Quick note", store.MailHumanReply, repliedAt, &f.acme.ID)
	f.act()

	if card := f.card(); card.ContactedAt == nil || !card.ContactedAt.Equal(repliedAt) {
		t.Fatalf("contacted at %v; want Ada's reply at %v, not the earlier message from someone else in the thread", card.ContactedAt, repliedAt)
	}
}

func TestARejectionForAClosedCardIsNoNews(t *testing.T) {
	f := startFixture(t)
	card := f.addCard("Closed")
	f.hub.MarkApplicationContacted(f.ctx, mailWatcher, card.ID, time.Now().Add(-48*time.Hour), "")

	f.receive("Ada <ada@acme.com>", "Re: Quick note", store.MailRejection, time.Now().Add(-time.Hour), &f.acme.ID)
	f.act()

	if len(f.updates.updates) != 0 {
		t.Fatalf("updates = %+v", f.updates.updates)
	}
}
