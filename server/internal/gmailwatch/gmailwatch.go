// Package gmailwatch hears Gmail announce mailbox changes through a Pub/Sub
// subscription, and records the messages each change adds.
package gmailwatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"

	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/mailmatch"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// renewalInterval follows Gmail's advice to renew a watch daily; it
	// lapses after 7 days.
	renewalInterval   = 24 * time.Hour
	retryDelay        = time.Minute
	maximumRetryDelay = 10 * time.Minute
)

// ignoredLabels mark messages that aren't mail sent or received.
var ignoredLabels = []string{"DRAFT", "SPAM", "CHAT"}

type mailbox interface {
	WatchMailbox(ctx context.Context, topic string) (google.MailboxWatch, error)
	ListAddedMessages(ctx context.Context, startHistoryID string) (google.MailboxChanges, error)
	GetMessageSummary(ctx context.Context, id string) (google.MessageSummary, error)
	ListMessageIDs(ctx context.Context, query string) ([]string, error)
	GetTokenSource(ctx context.Context) (oauth2.TokenSource, error)
}

type Watcher struct {
	hub          *store.Store
	mailbox      mailbox
	topic        string
	subscription string
	// OnMailRecorded, when set, is called after new mail was recorded.
	OnMailRecorded func()
}

// New watches the mailbox through topic and subscription, full Pub/Sub
// names such as "projects/p/topics/t" and "projects/p/subscriptions/s".
func New(hub *store.Store, mailbox mailbox, topic, subscription string) *Watcher {
	return &Watcher{hub: hub, mailbox: mailbox, topic: topic, subscription: subscription}
}

// Run keeps the watch renewed and reads the mailbox's changes each time
// Gmail announces some, and once at start to catch up, until ctx ends.
func (watcher *Watcher) Run(ctx context.Context) {
	announcements := make(chan struct{}, 1)
	go watcher.receive(ctx, announcements)
	renewal := time.NewTimer(0)
	defer renewal.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-renewal.C:
			if err := watcher.RenewWatch(ctx); err != nil {
				slog.Warn("gmail watch not renewed", "error", err, "retry in", retryDelay.String())
				renewal.Reset(retryDelay)
				continue
			}
			renewal.Reset(renewalInterval)
		case <-announcements:
		}
		watcher.syncAndLog(ctx)
	}
}

// RenewWatch asks Gmail to keep announcing changes, and stores when the
// watch lapses.
func (watcher *Watcher) RenewWatch(ctx context.Context) error {
	watch, err := watcher.mailbox.WatchMailbox(ctx, watcher.topic)
	if err != nil {
		return err
	}
	return watcher.hub.SaveGmailWatch(ctx, watch.HistoryID, watch.ExpiresAt)
}

// Sync records the messages added since reading last stopped and matches
// them to what the hub knows, then stores where the next reading resumes. A failure leaves that point where it was,
// so the next sync reads the same changes again; recording is idempotent.
// When Gmail no longer keeps changes that far back, reading resumes from now
// and the mail in between goes unrecorded.
func (watcher *Watcher) Sync(ctx context.Context) (recorded int, err error) {
	connection, err := watcher.hub.GetGoogleConnection(ctx)
	if err != nil {
		return 0, err
	}
	if connection.GmailHistoryID == "" {
		return 0, errors.New("the mailbox isn't watched yet")
	}
	changes, err := watcher.mailbox.ListAddedMessages(ctx, connection.GmailHistoryID)
	if errors.Is(err, google.ErrHistoryTooOld) {
		watch, err := watcher.mailbox.WatchMailbox(ctx, watcher.topic)
		if err != nil {
			return 0, err
		}
		slog.Warn("gmail changes lost: reading resumes from now", "from history", connection.GmailHistoryID, "to", watch.HistoryID)
		if err := watcher.hub.SaveGmailHistoryID(ctx, watch.HistoryID); err != nil {
			return 0, err
		}
		return 0, watcher.hub.SaveGmailWatch(ctx, watch.HistoryID, watch.ExpiresAt)
	}
	if err != nil {
		return 0, err
	}
	messages, err := watcher.recordMessages(ctx, changes.AddedMessageIDs)
	if err != nil {
		return len(messages), err
	}
	if _, err := mailmatch.MatchMessages(ctx, watcher.hub, messages); err != nil {
		return len(messages), err
	}
	watcher.announceRecorded(len(messages))
	return len(messages), watcher.hub.SaveGmailHistoryID(ctx, changes.HistoryID)
}

// BackfillResult counts what a backfill recorded and matched.
type BackfillResult struct {
	Recorded int `json:"recorded"`
	Matched  int `json:"matched"`
}

// Backfill records the mail of the last days that isn't recorded yet,
// leaving out promotions and social mail, then matches every message still
// unmatched, so a company added since picks up its old mail.
func (watcher *Watcher) Backfill(ctx context.Context, days int) (BackfillResult, error) {
	ids, err := watcher.mailbox.ListMessageIDs(ctx, fmt.Sprintf("newer_than:%dd -category:promotions -category:social", days))
	if err != nil {
		return BackfillResult{}, err
	}
	var newIDs []string
	for _, id := range ids {
		recorded, err := watcher.hub.IsMailMessageRecorded(ctx, id)
		if err != nil {
			return BackfillResult{}, err
		}
		if !recorded {
			newIDs = append(newIDs, id)
		}
	}
	messages, err := watcher.recordMessages(ctx, newIDs)
	result := BackfillResult{Recorded: len(messages)}
	if err != nil {
		return result, err
	}
	result.Matched, err = mailmatch.MatchUnmatched(ctx, watcher.hub)
	watcher.announceRecorded(result.Recorded + result.Matched)
	return result, err
}

func (watcher *Watcher) announceRecorded(count int) {
	if count > 0 && watcher.OnMailRecorded != nil {
		watcher.OnMailRecorded()
	}
}

// recordMessages stores the messages of ids that are mail sent or received,
// skipping any deleted since, and returns the ones newly recorded.
func (watcher *Watcher) recordMessages(ctx context.Context, ids []string) ([]store.MailMessage, error) {
	var recorded []store.MailMessage
	for _, id := range ids {
		summary, err := watcher.mailbox.GetMessageSummary(ctx, id)
		if errors.Is(err, google.ErrMessageNotFound) {
			continue
		}
		if err != nil {
			return recorded, err
		}
		if slices.ContainsFunc(summary.LabelIDs, func(label string) bool { return slices.Contains(ignoredLabels, label) }) {
			continue
		}
		message, created, err := watcher.hub.RecordMailMessage(ctx, convertSummaryToNewMailMessage(summary))
		if err != nil {
			return recorded, err
		}
		if created {
			recorded = append(recorded, message)
		}
	}
	return recorded, nil
}

func (watcher *Watcher) syncAndLog(ctx context.Context) {
	recorded, err := watcher.Sync(ctx)
	if err != nil {
		slog.Warn("gmail changes not read", "error", err)
		return
	}
	if recorded > 0 {
		slog.Info("gmail changes read", "recorded", recorded)
	}
}

// receive pulls Gmail's announcements from the subscription and passes them
// on; many announcements while a sync runs make one more sync. The
// announcement itself is only a nudge: the history ID stored is what says
// where reading resumes, so it is acknowledged at once.
func (watcher *Watcher) receive(ctx context.Context, announcements chan<- struct{}) {
	delay := retryDelay
	for ctx.Err() == nil {
		startedAt := time.Now()
		err := watcher.receiveUntilStopped(ctx, announcements)
		if ctx.Err() != nil {
			return
		}
		if time.Since(startedAt) > maximumRetryDelay {
			delay = retryDelay
		}
		slog.Warn("gmail announcements stopped", "error", err, "retry in", delay.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, maximumRetryDelay)
	}
}

func (watcher *Watcher) receiveUntilStopped(ctx context.Context, announcements chan<- struct{}) error {
	project, err := parseProject(watcher.subscription)
	if err != nil {
		return err
	}
	tokens, err := watcher.mailbox.GetTokenSource(ctx)
	if err != nil {
		return err
	}
	client, err := pubsub.NewClient(ctx, project, option.WithTokenSource(tokens))
	if err != nil {
		return err
	}
	defer client.Close()
	slog.Info("gmail announcements on", "subscription", watcher.subscription)
	return client.Subscriber(watcher.subscription).Receive(ctx, func(_ context.Context, message *pubsub.Message) {
		select {
		case announcements <- struct{}{}:
		default:
		}
		message.Ack()
	})
}

// parseProject reads the project out of a full subscription name.
func parseProject(subscription string) (string, error) {
	parts := strings.Split(subscription, "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "subscriptions" || parts[1] == "" || parts[3] == "" {
		return "", fmt.Errorf("%q is not a full subscription name such as projects/p/subscriptions/s", subscription)
	}
	return parts[1], nil
}

func convertSummaryToNewMailMessage(summary google.MessageSummary) store.NewMailMessage {
	direction := store.MailReceived
	if slices.Contains(summary.LabelIDs, "SENT") {
		direction = store.MailSent
	}
	var recipients []string
	for _, header := range []string{summary.To, summary.Cc} {
		if header != "" {
			recipients = append(recipients, header)
		}
	}
	return store.NewMailMessage{
		GmailMessageID: summary.ID, ThreadID: summary.ThreadID, Direction: direction, Sender: summary.From,
		Recipients: strings.Join(recipients, ", "), Subject: summary.Subject, SentAt: summary.Date, LabelIDs: summary.LabelIDs,
	}
}
