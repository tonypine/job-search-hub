// Package mailtriage sorts each received message into a mail class: rules
// sort the obvious senders, and the local model reads the job-related rest
// through the editable mail_triage prompt.
package mailtriage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

const (
	// maximumMessagesPerPass bounds one pass; the next pass picks up the rest.
	maximumMessagesPerPass = 500
	// maximumTextLength keeps a message within the model's context.
	maximumTextLength   = 6000
	maximumAnswerTokens = 512
)

// jobAlertSenders send lists of openings.
var jobAlertSenders = []string{"match.indeed.com", "jobs-noreply@linkedin.com", "jobalerts-noreply@linkedin.com", "noreply@glassdoor.com"}

// jobBoardApplicationSenders confirm applications sent through a job board.
var jobBoardApplicationSenders = []string{"indeedapply@indeed.com"}

// directMessageSenders relay a person's message, where recruiters write first.
var directMessageSenders = []string{"messaging-digest-noreply@linkedin.com", "messages-noreply@linkedin.com", "inmail-hit-reply@linkedin.com"}

// bulkCategories are the Gmail categories of marketing and social network
// mail. Gmail files people's mail under Primary or Updates.
var bulkCategories = []string{"CATEGORY_PROMOTIONS", "CATEGORY_SOCIAL"}

// confirmationSubjects are subjects only an application confirmation has.
var confirmationSubjects = []string{
	"received your application", "application received", "thank you for applying", "thanks for applying",
	"recebemos sua candidatura", "candidatura recebida", "inscricao via indeed",
}

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

type mailbox interface {
	GetMessage(ctx context.Context, id string) (google.Message, error)
}

type Classifier struct {
	hub     *store.Store
	mailbox mailbox
	client  modelClient
	nudges  chan struct{}
	// OnClassified, when set, is called after a pass classified any mail.
	OnClassified func()
}

func NewClassifier(hub *store.Store, mailbox mailbox, client modelClient) *Classifier {
	return &Classifier{hub: hub, mailbox: mailbox, client: client, nudges: make(chan struct{}, 1)}
}

// Nudge asks for a pass now, as when new mail was recorded.
func (classifier *Classifier) Nudge() {
	select {
	case classifier.nudges <- struct{}{}:
	default:
	}
}

// PassSummary counts one pass over the mail awaiting a class.
type PassSummary struct {
	ByRule  int
	ByModel int
	Failed  int
}

// Run classifies once at start, then every interval and on each nudge,
// until ctx ends.
func (classifier *Classifier) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		summary, err := classifier.ClassifyOnce(ctx)
		if err != nil {
			slog.Error("mail triage pass stopped", "error", err, "by rule", summary.ByRule, "by model", summary.ByModel)
		} else if summary != (PassSummary{}) {
			slog.Info("mail triage pass done", "by rule", summary.ByRule, "by model", summary.ByModel, "failed", summary.Failed)
		}
		if summary.ByRule+summary.ByModel > 0 && classifier.OnClassified != nil {
			classifier.OnClassified()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-classifier.nudges:
		}
	}
}

// ClassifyOnce classifies every received message awaiting a class: first
// all the rules can sort, so a model outage holds none of them up, then the
// rest through the model. A message the model fails on is logged, counted
// and left for the next pass; an unreachable model server stops the pass.
func (classifier *Classifier) ClassifyOnce(ctx context.Context) (PassSummary, error) {
	messages, err := classifier.hub.ListMailAwaitingClassification(ctx, maximumMessagesPerPass)
	if err != nil {
		return PassSummary{}, err
	}
	var summary PassSummary
	var forModel []store.MailMessage
	for _, message := range messages {
		classification, found := ClassifyByRule(message)
		if !found {
			forModel = append(forModel, message)
			continue
		}
		if err := classifier.hub.SaveMailClassification(ctx, message.ID, classification); err != nil {
			return summary, err
		}
		summary.ByRule++
	}
	if len(forModel) == 0 {
		return summary, nil
	}
	prompt, err := classifier.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindMailTriage)
	if err != nil {
		return summary, fmt.Errorf("read the mail_triage prompt: %w", err)
	}
	for _, message := range forModel {
		classification, err := classifier.classifyByModel(ctx, message, prompt)
		if errors.Is(err, chatcompletions.ErrUnreachable) || ctx.Err() != nil {
			return summary, err
		}
		if err == nil {
			err = classifier.hub.SaveMailClassification(ctx, message.ID, classification)
		}
		if err != nil {
			summary.Failed++
			slog.Warn("mail triage failed", "message", message.GmailMessageID, "subject", message.Subject, "error", err)
			continue
		}
		if classification.ClassifiedBy == store.ClassifiedByModel {
			summary.ByModel++
		} else {
			summary.ByRule++
		}
	}
	return summary, nil
}

// ClassifyByRule sorts the mail rules can: job alerts, application
// confirmations, and promotions or social mail about no company the hub
// knows, which is noise. found is false for mail the model must read: mail
// about a company the hub knows, relayed direct messages, and the rest of the
// mail outside the bulk categories, where a recruiter the hub doesn't know
// yet may write.
func ClassifyByRule(message store.MailMessage) (store.MailClassification, bool) {
	sender := strings.ToLower(message.Sender)
	byRule := func(class, reason string) (store.MailClassification, bool) {
		return store.MailClassification{Class: class, ClassifiedBy: store.ClassifiedByRule, Reason: reason}, true
	}
	switch {
	case containsAny(sender, jobAlertSenders):
		return byRule(store.MailJobAlert, "sent by a job board's alerts")
	case containsAny(sender, jobBoardApplicationSenders):
		return byRule(store.MailApplicationConfirmation, "sent by a job board when an application goes through")
	case hasConfirmationSubject(message.Subject) && IsAutomatedSender(sender):
		return byRule(store.MailApplicationConfirmation, "an automated sender with an application-received subject")
	case message.CompanyID != nil, containsAny(sender, directMessageSenders), !isInBulkCategory(message.LabelIDs):
		return store.MailClassification{}, false
	default:
		return byRule(store.MailNoise, "promotions or social mail, not about a company the hub knows")
	}
}

func (classifier *Classifier) classifyByModel(ctx context.Context, message store.MailMessage, prompt store.AgentPrompt) (store.MailClassification, error) {
	full, err := classifier.mailbox.GetMessage(ctx, message.GmailMessageID)
	if errors.Is(err, google.ErrMessageNotFound) {
		return store.MailClassification{Class: store.MailNoise, ClassifiedBy: store.ClassifiedByRule, Reason: "deleted from Gmail before it was read"}, nil
	}
	if err != nil {
		return store.MailClassification{}, err
	}
	companyName := ""
	if message.CompanyID != nil {
		company, err := classifier.hub.GetCompany(ctx, *message.CompanyID)
		if err != nil {
			return store.MailClassification{}, err
		}
		companyName = company.Name
	}
	answer, err := classifier.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		System: prompt.Body, User: formatMessageText(message, full.Text, companyName),
		SchemaName: store.AgentPromptKindMailTriage, Schema: prompt.ResultSchema, Examples: prompt.Examples, MaxTokens: maximumAnswerTokens,
		Task: chatcompletions.TaskLabel{SubjectID: &message.ID, PromptID: &prompt.ID, PromptVersion: prompt.Version},
	})
	if err != nil {
		return store.MailClassification{}, err
	}
	var parsed struct {
		Class  string `json:"class"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(answer.Object, &parsed); err != nil {
		return store.MailClassification{}, fmt.Errorf("read the model's answer: %w", err)
	}
	return store.MailClassification{Class: parsed.Class, ClassifiedBy: store.ClassifiedByModel, Reason: parsed.Reason, PromptID: &prompt.ID}, nil
}

// formatMessageText is what the model reads: the headers, the company the
// hub matched, and the text.
func formatMessageText(message store.MailMessage, text, companyName string) string {
	var formatted strings.Builder
	fmt.Fprintf(&formatted, "From: %s\nTo: %s\nSubject: %s\nDate: %s\n", message.Sender, message.Recipients, message.Subject,
		message.SentAt.Format(time.RFC1123))
	if companyName != "" {
		fmt.Fprintf(&formatted, "Matched company: %s\n", companyName)
	}
	if len(text) > maximumTextLength {
		text = strings.ToValidUTF8(text[:maximumTextLength], "")
	}
	fmt.Fprintf(&formatted, "\nText:\n%s", text)
	return formatted.String()
}

func isInBulkCategory(labelIDs []string) bool {
	return slices.ContainsFunc(labelIDs, func(label string) bool { return slices.Contains(bulkCategories, label) })
}

func containsAny(text string, parts []string) bool {
	return slices.ContainsFunc(parts, func(part string) bool { return strings.Contains(text, part) })
}

func hasConfirmationSubject(subject string) bool {
	normalized := wordmatch.Normalize(subject)
	return slices.ContainsFunc(confirmationSubjects, func(phrase string) bool { return wordmatch.Contains(normalized, phrase) })
}

// IsAutomatedSender reports whether a sender is an address nobody reads.
func IsAutomatedSender(sender string) bool {
	return containsAny(strings.ToLower(sender), []string{"no-reply", "noreply", "do-not-reply", "donotreply", "nao-responda", "naoresponda", "notifications@"})
}
