package gmailwatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

// fakeMailbox answers from its fields; a history ID in tooOld is one Gmail
// no longer keeps.
type fakeMailbox struct {
	watch     google.MailboxWatch
	changes   map[string]google.MailboxChanges
	tooOld    string
	summaries map[string]google.MessageSummary
	failing   string
}

func (mailbox *fakeMailbox) WatchMailbox(context.Context, string) (google.MailboxWatch, error) {
	return mailbox.watch, nil
}

func (mailbox *fakeMailbox) ListAddedMessages(_ context.Context, startHistoryID string) (google.MailboxChanges, error) {
	if startHistoryID == mailbox.tooOld {
		return google.MailboxChanges{}, google.ErrHistoryTooOld
	}
	return mailbox.changes[startHistoryID], nil
}

func (mailbox *fakeMailbox) GetMessageSummary(_ context.Context, id string) (google.MessageSummary, error) {
	if id == mailbox.failing {
		return google.MessageSummary{}, errors.New("gmail is down")
	}
	summary, found := mailbox.summaries[id]
	if !found {
		return google.MessageSummary{}, google.ErrMessageNotFound
	}
	return summary, nil
}

func (mailbox *fakeMailbox) GetTokenSource(context.Context) (oauth2.TokenSource, error) {
	return nil, errors.New("no Pub/Sub in tests")
}

func startWatcher(t *testing.T, mailbox *fakeMailbox) (*Watcher, *store.Store) {
	t.Helper()
	hub := store.New(testdatabase.New(t))
	if _, err := hub.SaveGoogleConnection(context.Background(), store.Actor{Kind: store.ActorOwner}, "owner@example.com", "refresh-1", google.Scopes); err != nil {
		t.Fatal(err)
	}
	return New(hub, mailbox, "projects/p/topics/t", "projects/p/subscriptions/s"), hub
}

func TestSyncRecordsTheMailAddedSinceTheWatchAndResumesAfterIt(t *testing.T) {
	sentAt := time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC)
	mailbox := &fakeMailbox{
		watch: google.MailboxWatch{HistoryID: "100", ExpiresAt: sentAt.Add(7 * 24 * time.Hour)},
		changes: map[string]google.MailboxChanges{
			"100": {AddedMessageIDs: []string{"reply", "sent", "draft", "deleted"}, HistoryID: "110"},
			"110": {HistoryID: "112"},
		},
		summaries: map[string]google.MessageSummary{
			"reply": {ID: "reply", ThreadID: "t1", From: "Ada <ada@acme.com>", To: "owner@example.com", Cc: "bob@acme.com",
				Subject: "Re: Engineer", Date: sentAt, LabelIDs: []string{"INBOX", "UNREAD"}},
			"sent":  {ID: "sent", ThreadID: "t1", From: "owner@example.com", To: "ada@acme.com", Subject: "Engineer", Date: sentAt, LabelIDs: []string{"SENT"}},
			"draft": {ID: "draft", ThreadID: "t2", Date: sentAt, LabelIDs: []string{"DRAFT"}},
		},
	}
	watcher, hub := startWatcher(t, mailbox)
	ctx := context.Background()

	if _, err := watcher.Sync(ctx); err == nil {
		t.Fatal("a sync before any watch should fail: there is nowhere to resume from")
	}
	if err := watcher.RenewWatch(ctx); err != nil {
		t.Fatal(err)
	}
	recorded, err := watcher.Sync(ctx)
	if err != nil || recorded != 2 {
		t.Fatalf("sync = %d, %v; want the reply and the sent message", recorded, err)
	}
	messages, _ := hub.ListMailMessages(ctx, 10)
	directions := map[string]string{}
	for _, message := range messages {
		directions[message.GmailMessageID] = message.Direction
	}
	if len(messages) != 2 || directions["reply"] != store.MailReceived || directions["sent"] != store.MailSent {
		t.Fatalf("messages = %+v", messages)
	}
	for _, message := range messages {
		if message.GmailMessageID == "reply" && message.Recipients != "owner@example.com, bob@acme.com" {
			t.Fatalf("recipients = %q", message.Recipients)
		}
	}

	connection, _ := hub.GetGoogleConnection(ctx)
	if connection.GmailHistoryID != "110" || connection.GmailWatchExpiresAt == nil {
		t.Fatalf("connection = %+v; want reading to resume at 110 and the watch's expiry kept", connection)
	}
	if err := watcher.RenewWatch(ctx); err != nil {
		t.Fatal(err)
	}
	if connection, _ := hub.GetGoogleConnection(ctx); connection.GmailHistoryID != "110" {
		t.Fatalf("a renewal moved the resume point to %q", connection.GmailHistoryID)
	}
	if recorded, err := watcher.Sync(ctx); err != nil || recorded != 0 {
		t.Fatalf("second sync = %d, %v", recorded, err)
	}
}

func TestAFailedSyncReadsTheSameChangesAgain(t *testing.T) {
	mailbox := &fakeMailbox{
		watch:     google.MailboxWatch{HistoryID: "100", ExpiresAt: time.Now().Add(time.Hour)},
		changes:   map[string]google.MailboxChanges{"100": {AddedMessageIDs: []string{"reply"}, HistoryID: "110"}},
		summaries: map[string]google.MessageSummary{"reply": {ID: "reply", ThreadID: "t1", Date: time.Now(), LabelIDs: []string{"INBOX"}}},
		failing:   "reply",
	}
	watcher, hub := startWatcher(t, mailbox)
	ctx := context.Background()
	if err := watcher.RenewWatch(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := watcher.Sync(ctx); err == nil {
		t.Fatal("want the Gmail failure")
	}
	if connection, _ := hub.GetGoogleConnection(ctx); connection.GmailHistoryID != "100" {
		t.Fatalf("resume point = %q after a failure; want 100", connection.GmailHistoryID)
	}
	mailbox.failing = ""
	if recorded, err := watcher.Sync(ctx); err != nil || recorded != 1 {
		t.Fatalf("retry = %d, %v", recorded, err)
	}
}

func TestReadingResumesFromNowWhenGmailNoLongerKeepsTheChanges(t *testing.T) {
	mailbox := &fakeMailbox{watch: google.MailboxWatch{HistoryID: "5", ExpiresAt: time.Now().Add(time.Hour)}, tooOld: "5"}
	watcher, hub := startWatcher(t, mailbox)
	ctx := context.Background()
	if err := watcher.RenewWatch(ctx); err != nil {
		t.Fatal(err)
	}
	mailbox.watch.HistoryID = "900"

	if recorded, err := watcher.Sync(ctx); err != nil || recorded != 0 {
		t.Fatalf("sync = %d, %v", recorded, err)
	}
	if connection, _ := hub.GetGoogleConnection(ctx); connection.GmailHistoryID != "900" {
		t.Fatalf("resume point = %q; want the mailbox's current 900", connection.GmailHistoryID)
	}
}

func TestSigningInAgainKeepsTheResumePointOnlyForTheSameAddress(t *testing.T) {
	mailbox := &fakeMailbox{watch: google.MailboxWatch{HistoryID: "100", ExpiresAt: time.Now().Add(time.Hour)}}
	watcher, hub := startWatcher(t, mailbox)
	ctx := context.Background()
	if err := watcher.RenewWatch(ctx); err != nil {
		t.Fatal(err)
	}
	owner := store.Actor{Kind: store.ActorOwner}

	hub.SaveGoogleConnection(ctx, owner, "owner@example.com", "refresh-2", google.Scopes)
	if connection, _ := hub.GetGoogleConnection(ctx); connection.GmailHistoryID != "100" {
		t.Fatalf("same address: resume point = %q", connection.GmailHistoryID)
	}
	hub.SaveGoogleConnection(ctx, owner, "other@example.com", "refresh-3", google.Scopes)
	if connection, _ := hub.GetGoogleConnection(ctx); connection.GmailHistoryID != "" || connection.GmailWatchExpiresAt != nil {
		t.Fatalf("another address: connection = %+v; want reading to start over", connection)
	}
}

func TestTheProjectIsReadFromTheSubscriptionName(t *testing.T) {
	if project, err := parseProject("projects/p-1/subscriptions/s"); err != nil || project != "p-1" {
		t.Fatalf("project = %q, %v", project, err)
	}
	if _, err := parseProject("s"); err == nil {
		t.Fatal("a bare subscription ID should be refused")
	}
}
