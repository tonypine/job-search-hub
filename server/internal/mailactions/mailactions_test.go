package mailactions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

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
	pool    *pgxpool.Pool
	hub     *store.Store
	handler *Handler
	updates *recordedUpdates
	phases  map[string]store.PipelinePhase
	acme    store.Company
}

func startFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdatabase.New(t)
	hub := store.New(pool)
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
	return &fixture{t: t, ctx: ctx, pool: pool, hub: hub, handler: NewHandler(hub, updates), updates: updates, phases: phases, acme: acme}
}

// receive records a classified message, about Acme when companyID is set.
func (f *fixture) receive(sender, subject, class string, sentAt time.Time, companyID *uuid.UUID) {
	f.t.Helper()
	f.record(store.MailReceived, sender, "owner@example.com", subject, "thread", sentAt, companyID, class)
}

// send records a message the owner sent in thread, to Acme when companyID
// is set.
func (f *fixture) send(recipients, subject, thread string, sentAt time.Time, companyID *uuid.UUID) store.MailMessage {
	f.t.Helper()
	return f.record(store.MailSent, "owner@example.com", recipients, subject, thread, sentAt, companyID, "")
}

func (f *fixture) record(direction, sender, recipients, subject, thread string, sentAt time.Time, companyID *uuid.UUID, class string) store.MailMessage {
	f.t.Helper()
	message, _, err := f.hub.RecordMailMessage(f.ctx, store.NewMailMessage{
		GmailMessageID: uuid.NewString(), ThreadID: thread, Direction: direction, Sender: sender, Recipients: recipients,
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
	return message
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
	if len(f.updates.updates) != 1 || f.updates.updates[0].Title != "Acme confirmed your application" ||
		!strings.Contains(f.updates.updates[0].Body, "dated the application") || !strings.Contains(f.updates.updates[0].SourceURL, "authuser=owner%40example.com#all/") {
		t.Fatalf("updates = %+v", f.updates.updates)
	}
	if acted := f.act(); acted != 0 || len(f.updates.updates) != 1 {
		t.Fatalf("a second pass acted on %d messages; each message is acted on once", acted)
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

	f.send("Ada <ada@acme.com>", "Checking in", "thread", sentAt, &f.acme.ID)
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

func TestAColdEmailToACompanyWithoutACardRecordsOutreach(t *testing.T) {
	f := startFixture(t)
	sentAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)

	message := f.send("Ada <ada@acme.com>", "Hello from a fellow engineer", "cold", sentAt, &f.acme.ID)
	f.act()

	card := f.card()
	if card.PhaseID != f.phases["Applied"].ID || card.JobID != nil || !card.PhaseEnteredAt.Equal(sentAt) {
		t.Fatalf("card = %+v; want an outreach card in Applied since the mail was sent", card)
	}
	if len(f.updates.updates) != 1 || f.updates.updates[0].Title != "You reached out to Acme" || f.updates.updates[0].Kind != "outreach" ||
		!strings.HasPrefix(f.updates.updates[0].Body, "Hello from a fellow engineer\nThe hub recorded the outreach") ||
		!strings.HasSuffix(f.updates.updates[0].SourceURL, "#all/"+message.GmailMessageID) || *f.updates.updates[0].CompanyID != f.acme.ID {
		t.Fatalf("updates = %+v", f.updates.updates)
	}
	var fromTheMail string
	var changes int
	err := f.pool.QueryRow(f.ctx, `
		SELECT coalesce(string_agg(operation, ',' ORDER BY id) FILTER (WHERE source_url = $2), ''), count(*) FROM changes WHERE entity_id = $1`,
		card.ID, f.updates.updates[0].SourceURL).Scan(&fromTheMail, &changes)
	if err != nil || fromTheMail != "create,move,outreach" || changes != 3 {
		t.Fatalf("changes from the mail = %q of %d, %v; want each change to name the mail", fromTheMail, changes, err)
	}

	f.send("Ada <ada@acme.com>", "Hello again", "cold-2", sentAt.Add(time.Minute), &f.acme.ID)
	f.act()
	if again := f.card(); again.ID != card.ID || again.LastFollowedUpAt == nil || len(f.updates.updates) != 1 {
		t.Fatalf("card = %+v; want a second cold email to count as a follow-up on the same card", again)
	}
}

func TestAColdEmailToACompanyWithAClosedCardStartsANewOne(t *testing.T) {
	f := startFixture(t)
	closed := f.addCard("Closed")

	f.send("ada@acme.com", "A different team", "cold", time.Now().Add(-time.Hour), &f.acme.ID)
	f.act()

	if card := f.card(); card.ID == closed.ID || card.PhaseID != f.phases["Applied"].ID {
		t.Fatalf("card = %+v; want a new outreach card in Applied", card)
	}
}

func TestMailAnsweringAThreadIsNotOutreach(t *testing.T) {
	f := startFixture(t)
	startedAt := time.Now().Add(-3 * time.Hour)

	f.receive("Ada <ada@acme.com>", "Coffee?", store.MailNoise, startedAt, &f.acme.ID)
	f.send("ada@acme.com", "Sounds good", "thread", startedAt.Add(time.Hour), &f.acme.ID)
	f.send("ada@acme.com", "Re: Your talk", "older-thread", startedAt.Add(2*time.Hour), &f.acme.ID)
	f.act()

	if _, err := f.hub.FindCompanyApplication(f.ctx, f.acme.ID); err == nil || len(f.updates.updates) != 0 {
		t.Fatalf("err = %v, updates = %+v; want no card for answers in a thread the company started", err, f.updates.updates)
	}
}

func TestMailMatchedOnlyByItsThreadIsNotOutreach(t *testing.T) {
	f := startFixture(t)
	message, _, err := f.hub.RecordMailMessage(f.ctx, store.NewMailMessage{
		GmailMessageID: "forward", ThreadID: "intro", Direction: store.MailSent, Sender: "owner@example.com",
		Recipients: "Pat <pat@example.dev>", Subject: "Meet Acme", SentAt: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.hub.SaveMailMatch(f.ctx, message.ID, store.MailMatch{CompanyID: f.acme.ID, MatchedBy: store.MatchedByThread}); err != nil {
		t.Fatal(err)
	}
	f.act()

	if _, err := f.hub.FindCompanyApplication(f.ctx, f.acme.ID); err == nil || len(f.updates.updates) != 0 {
		t.Fatalf("err = %v, updates = %+v; want no card for mail to someone outside the company", err, f.updates.updates)
	}
}

func TestAReplyIsKnownByItsSubject(t *testing.T) {
	for subject, want := range map[string]bool{"Re: Engineer": true, "RES: Vaga": true, " re:x": true, "Regarding the role": false, "Fwd: CV": false} {
		if got := isReply(subject); got != want {
			t.Fatalf("%q: got %v, want %v", subject, got, want)
		}
	}
}
