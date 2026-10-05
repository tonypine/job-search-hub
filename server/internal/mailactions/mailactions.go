// Package mailactions acts on classified mail for the owner, who chose to
// have the hub act and then tell: it moves the company's card, notes contacts
// and follow-ups, and records an update saying what it did.
package mailactions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/mailmatch"
	"github.com/tonypine/job-search-hub/server/internal/mailtriage"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumMessagesPerPass bounds one pass; the next pass picks up the rest.
	maximumMessagesPerPass = 500
	// newsAge is how old mail can be and still make an update when it
	// changed nothing.
	newsAge = 7 * 24 * time.Hour
	// outreachKind is the kind of update a cold message the hub counted
	// records.
	outreachKind = "outreach"
)

// replyPrefixes start the subject of mail answering a thread, in the
// languages the owner's mail comes in.
var replyPrefixes = []string{"re:", "res:", "aw:", "sv:"}

// Phases the hub moves cards to, by name, since the owner can rename them.
const (
	appliedPhase      = "applied"
	inContactPhase    = "in contact"
	interviewingPhase = "interviewing"
)

// mailWatcher is who the change log says acted; each change's source is the
// message that caused it.
var mailWatcher = store.Actor{Kind: store.ActorSystem}

type updateRecorder interface {
	Record(ctx context.Context, input store.NewUpdate) (store.Update, error)
}

type Handler struct {
	hub     *store.Store
	updates updateRecorder
	nudges  chan struct{}
}

func NewHandler(hub *store.Store, updates updateRecorder) *Handler {
	return &Handler{hub: hub, updates: updates, nudges: make(chan struct{}, 1)}
}

// Nudge asks for a pass now, as when mail was classified.
func (handler *Handler) Nudge() {
	select {
	case handler.nudges <- struct{}{}:
	default:
	}
}

// Run acts once at start, then every interval and on each nudge, until ctx
// ends.
func (handler *Handler) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		acted, err := handler.ActOnce(ctx)
		if err != nil {
			slog.Error("mail actions stopped", "error", err, "acted", acted)
		} else if acted > 0 {
			slog.Info("mail actions done", "acted", acted)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-handler.nudges:
		}
	}
}

// ActOnce acts on every message awaiting it, oldest first, so cards move in
// the order the mail came. It returns how many it acted on.
func (handler *Handler) ActOnce(ctx context.Context) (int, error) {
	messages, err := handler.hub.ListMailAwaitingAction(ctx, maximumMessagesPerPass)
	if err != nil || len(messages) == 0 {
		return 0, err
	}
	phases, err := handler.hub.ListPipelinePhases(ctx)
	if err != nil {
		return 0, err
	}
	connection, err := handler.hub.GetGoogleConnection(ctx)
	if err != nil {
		return 0, err
	}
	for index, message := range messages {
		pass := messagePass{handler: handler, phases: phases, message: message, sourceURL: getGmailLink(connection.Email, message.GmailMessageID)}
		action, err := pass.act(ctx)
		if err != nil {
			return index, fmt.Errorf("act on %q: %w", message.Subject, err)
		}
		if err := handler.hub.MarkMailActed(ctx, message.ID, action); err != nil {
			return index, err
		}
	}
	return len(messages), nil
}

// messagePass is the acting on one message.
type messagePass struct {
	handler   *Handler
	phases    []store.PipelinePhase
	message   store.MailMessage
	sourceURL string
	done      []string
}

// act does what the message calls for and returns what was done, in words.
func (pass *messagePass) act(ctx context.Context) (string, error) {
	message := pass.message
	if message.Direction == store.MailSent {
		return pass.actOnSentMail(ctx)
	}
	application, found, err := pass.findApplication(ctx)
	if err != nil {
		return "", err
	}
	switch message.Classification {
	case store.MailApplicationConfirmation:
		err = pass.confirmApplication(ctx, application, found)
	case store.MailHumanReply:
		err = pass.moveForward(ctx, application, found, inContactPhase)
	case store.MailInterviewInvite:
		err = pass.moveForward(ctx, application, found, interviewingPhase)
	case store.MailRecruiterOutreach:
		if !found && message.CompanyID == nil {
			application, found, err = pass.addRecruiterCompany(ctx)
			if err != nil {
				break
			}
		}
		err = pass.moveForward(ctx, application, found, inContactPhase)
	case store.MailRejection:
		err = pass.close(ctx, application, found)
	}
	if err != nil {
		return "", err
	}
	if application, found, err = pass.findApplication(ctx); err != nil {
		return "", err
	}
	if found && pass.isFromThePeopleAtTheCompany() && (application.ContactedAt == nil || application.ContactedAt.After(message.SentAt)) {
		if _, err := pass.handler.hub.MarkApplicationContacted(ctx, mailWatcher, application.ID, message.SentAt, pass.sourceURL); err != nil {
			return "", err
		}
		pass.done = append(pass.done, "noted the first contact")
	}
	return pass.announce(ctx, application, found)
}

// actOnSentMail counts mail the owner sent to a company with an open card as
// a follow-up, and a thread the owner starts with someone at a company
// without one as outreach.
func (pass *messagePass) actOnSentMail(ctx context.Context) (string, error) {
	application, found, err := pass.findApplication(ctx)
	if err != nil {
		return "", err
	}
	if !found || pass.isClosed(application) {
		return pass.recordOutreach(ctx)
	}
	if _, err := pass.handler.hub.RecordFollowUpAsOf(ctx, mailWatcher, application.ID, "Sent: "+pass.message.Subject, pass.message.SentAt, pass.sourceURL); err != nil {
		return "", err
	}
	return "recorded a follow-up", nil
}

// recordOutreach counts mail the owner wrote to someone at the company, as
// their address or the company's domain shows, as a cold message: the
// company's outreach card goes to Applied as of the send time, so its
// follow-up falls due, and an update says so. Mail answering a thread is
// not outreach, even when the hub never saw the thread's start.
func (pass *messagePass) recordOutreach(ctx context.Context) (string, error) {
	message := pass.message
	matchedByRecipient := message.MatchedBy == store.MatchedByPerson || message.MatchedBy == store.MatchedByDomain
	applied, known := pass.findPhase(appliedPhase)
	if !matchedByRecipient || !known || isReply(message.Subject) {
		return "nothing: no open card", nil
	}
	startsItsThread, err := pass.handler.hub.StartsItsThread(ctx, message)
	if err != nil || !startsItsThread {
		return "nothing: no open card", err
	}
	application, _, err := pass.handler.hub.RecordOutreach(ctx, mailWatcher, *message.CompanyID, "Sent: "+message.Subject, message.SentAt, pass.sourceURL)
	if err != nil {
		return "", err
	}
	company, err := pass.handler.hub.GetCompany(ctx, *message.CompanyID)
	if err != nil {
		return "", err
	}
	outcome := "The hub recorded the outreach and put the card in " + applied.Name + "."
	if applied.FollowUpDays != nil {
		outcome += " Follow up by " + message.SentAt.AddDate(0, 0, *applied.FollowUpDays).Format("Jan 2") + "."
	}
	if _, err := pass.handler.updates.Record(ctx, store.NewUpdate{
		Kind: outreachKind, Title: "You reached out to " + company.Name, Body: message.Subject + "\n" + outcome,
		JobID: application.JobID, CompanyID: message.CompanyID, SourceURL: pass.sourceURL,
	}); err != nil {
		return "", err
	}
	return "recorded outreach", nil
}

// confirmApplication puts the card in Applied at the mail's date: a card
// before Applied moves there, one in Applied since a later date gets the
// mail's date, and a company without a card gets one.
func (pass *messagePass) confirmApplication(ctx context.Context, application store.Application, found bool) error {
	applied, known := pass.findPhase(appliedPhase)
	if !known {
		return nil
	}
	if found && application.PhaseID == applied.ID {
		_, corrected, err := pass.handler.hub.CorrectApplicationPhaseEnteredAt(ctx, mailWatcher, application.ID, pass.message.SentAt, pass.sourceURL)
		if corrected {
			pass.done = append(pass.done, "dated the application "+pass.message.SentAt.Format("Jan 2"))
		}
		return err
	}
	return pass.moveForward(ctx, application, found, appliedPhase)
}

// moveForward moves the card to the named phase when that is forward of
// where it is; a company without a card gets one there. A card never moves
// backwards, and a closed one stays closed.
func (pass *messagePass) moveForward(ctx context.Context, application store.Application, found bool, phaseName string) error {
	target, known := pass.findPhase(phaseName)
	if !known || (pass.message.CompanyID == nil && !found) {
		return nil
	}
	if !found {
		added, _, err := pass.handler.hub.AddApplication(ctx, mailWatcher, store.ApplicationInput{CompanyID: pass.message.CompanyID, SourceURL: pass.sourceURL})
		if err != nil {
			return err
		}
		application = added
		pass.done = append(pass.done, "added a card")
	}
	current, _ := pass.findPhaseByID(application.PhaseID)
	if current.IsClosed || current.Position >= target.Position {
		return nil
	}
	if _, err := pass.handler.hub.MoveApplicationAsOf(ctx, mailWatcher, application.ID, target.ID, "", pass.message.SentAt, pass.sourceURL); err != nil {
		return err
	}
	pass.done = append(pass.done, "moved the card to "+target.Name)
	return nil
}

// close moves an open card to the closed phase, with the subject as reason.
func (pass *messagePass) close(ctx context.Context, application store.Application, found bool) error {
	if !found || pass.isClosed(application) {
		return nil
	}
	for _, phase := range pass.phases {
		if !phase.IsClosed {
			continue
		}
		reason := "Rejected by mail: " + pass.message.Subject
		if _, err := pass.handler.hub.MoveApplicationAsOf(ctx, mailWatcher, application.ID, phase.ID, reason, pass.message.SentAt, pass.sourceURL); err != nil {
			return err
		}
		pass.done = append(pass.done, "closed the card")
		return nil
	}
	return nil
}

// addRecruiterCompany stores the company a recruiter writes from, and the
// recruiter, when they write from a company's own address.
func (pass *messagePass) addRecruiterCompany(ctx context.Context) (store.Application, bool, error) {
	addresses := mailmatch.GetAddresses(pass.message.Sender)
	if len(addresses) == 0 || !pass.isFromAPerson() || !mailmatch.IsCompanyAddress(addresses[0]) {
		return store.Application{}, false, nil
	}
	domain := mailmatch.GetDomain(addresses[0])
	company, created, err := pass.handler.hub.CreateCompany(ctx, mailWatcher, store.NewCompany{
		Name: getCompanyNameFromDomain(domain), Domain: domain, FoundVia: "Recruiter outreach by mail", SourceURL: pass.sourceURL,
	})
	if err != nil {
		return store.Application{}, false, err
	}
	if created {
		pass.done = append(pass.done, "added "+company.Name)
	}
	if name := pass.getSenderName(); name != "" {
		if _, _, err := pass.handler.hub.AddPerson(ctx, mailWatcher, store.PersonInput{
			CompanyID: company.ID, Name: name, Relevance: "recruiter", SourceURL: pass.sourceURL, Email: addresses[0],
		}); err != nil {
			return store.Application{}, false, err
		}
	}
	if err := pass.handler.hub.SaveMailCompanyFromItsSender(ctx, pass.message.ID, company.ID); err != nil {
		return store.Application{}, false, err
	}
	pass.message.CompanyID = &company.ID
	pass.message.MatchedBy = store.MatchedByDomain
	return store.Application{}, false, nil
}

// announce records an update about the message and what was done, and
// returns what was done. Mail that changed nothing records no update when it
// is old, as a backfill finds, or when it only confirms what the card shows:
// a confirmation or a rejection for a card already there.
func (pass *messagePass) announce(ctx context.Context, application store.Application, found bool) (string, error) {
	message := pass.message
	confirmsTheCard := found && (message.Classification == store.MailApplicationConfirmation || message.Classification == store.MailRejection)
	if len(pass.done) == 0 && (time.Since(message.SentAt) > newsAge || confirmsTheCard) {
		return "nothing to change", nil
	}
	companyName := ""
	if message.CompanyID != nil {
		company, err := pass.handler.hub.GetCompany(ctx, *message.CompanyID)
		if err != nil {
			return "", err
		}
		companyName = company.Name
	}
	about := companyName
	if about == "" {
		about = pass.getSenderName()
	}
	var title string
	switch message.Classification {
	case store.MailHumanReply:
		title = fmt.Sprintf("%s replied", firstNonEmpty(pass.getSenderName(), about))
	case store.MailApplicationConfirmation:
		title = "Your application was confirmed"
		if companyName != "" {
			title = companyName + " confirmed your application"
		}
	case store.MailRejection:
		title = fmt.Sprintf("%s isn't moving forward", firstNonEmpty(about, "A company"))
	case store.MailInterviewInvite:
		title = fmt.Sprintf("%s invited you to interview", firstNonEmpty(about, "A company"))
	case store.MailRecruiterOutreach:
		title = fmt.Sprintf("%s reached out", firstNonEmpty(pass.getSenderName(), about, "A recruiter"))
	}
	action, outcome := "nothing to change", "The hub found nothing to change."
	if len(pass.done) > 0 {
		action = strings.Join(pass.done, "; ")
		outcome = "The hub " + action + "."
	}
	update := store.NewUpdate{
		Kind: message.Classification, Title: title, Body: message.Subject + "\n" + outcome, CompanyID: message.CompanyID, SourceURL: pass.sourceURL,
	}
	if found {
		update.JobID = application.JobID
	}
	if _, err := pass.handler.updates.Record(ctx, update); err != nil {
		return "", err
	}
	return action, nil
}

// findApplication returns the card of the company the message is about.
func (pass *messagePass) findApplication(ctx context.Context) (store.Application, bool, error) {
	if pass.message.CompanyID == nil {
		return store.Application{}, false, nil
	}
	application, err := pass.handler.hub.FindCompanyApplication(ctx, *pass.message.CompanyID)
	if errors.Is(err, store.ErrApplicationNotFound) {
		return store.Application{}, false, nil
	}
	return application, err == nil, err
}

func (pass *messagePass) findPhase(name string) (store.PipelinePhase, bool) {
	for _, phase := range pass.phases {
		if strings.EqualFold(strings.TrimSpace(phase.Name), name) {
			return phase, true
		}
	}
	return store.PipelinePhase{}, false
}

func (pass *messagePass) findPhaseByID(id uuid.UUID) (store.PipelinePhase, bool) {
	for _, phase := range pass.phases {
		if phase.ID == id {
			return phase, true
		}
	}
	return store.PipelinePhase{}, false
}

func (pass *messagePass) isClosed(application store.Application) bool {
	phase, _ := pass.findPhaseByID(application.PhaseID)
	return phase.IsClosed
}

// isFromThePeopleAtTheCompany reports whether a person at the company wrote
// the message, known by their address or the company's domain; only that
// counts as a contact, not someone else writing in the same thread.
func (pass *messagePass) isFromThePeopleAtTheCompany() bool {
	matchedBySender := pass.message.MatchedBy == store.MatchedByPerson || pass.message.MatchedBy == store.MatchedByDomain
	return matchedBySender && pass.isFromAPerson()
}

// isFromAPerson reports whether someone wrote the message, rather than a
// system.
func (pass *messagePass) isFromAPerson() bool {
	return pass.message.Direction == store.MailReceived && pass.message.Classification != store.MailApplicationConfirmation &&
		!mailtriage.IsAutomatedSender(pass.message.Sender)
}

// getSenderName returns the sender's name without a relay's "via LinkedIn".
func (pass *messagePass) getSenderName() string {
	name, _, _ := strings.Cut(mailmatch.GetDisplayName(pass.message.Sender), " via ")
	return strings.TrimSpace(name)
}

// getGmailLink opens the message in Gmail, in the owner's account.
func getGmailLink(ownerEmail, gmailMessageID string) string {
	return "https://mail.google.com/mail/?authuser=" + url.QueryEscape(ownerEmail) + "#all/" + gmailMessageID
}

// getCompanyNameFromDomain names a company after its domain until someone
// names it better: "talent.acme.io" is "Acme", "acme.com.br" is "Acme".
func getCompanyNameFromDomain(domain string) string {
	labels := strings.Split(domain, ".")
	index := len(labels) - 2
	if len(labels) >= 3 && len(labels[len(labels)-1]) == 2 && (labels[len(labels)-2] == "com" || labels[len(labels)-2] == "co") {
		index = len(labels) - 3
	}
	if index < 0 {
		return domain
	}
	name := labels[index]
	return strings.ToUpper(name[:1]) + name[1:]
}

// isReply reports whether a subject answers a thread: "Re: Engineer".
func isReply(subject string) bool {
	subject = strings.ToLower(strings.TrimSpace(subject))
	return slices.ContainsFunc(replyPrefixes, func(prefix string) bool { return strings.HasPrefix(subject, prefix) })
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
