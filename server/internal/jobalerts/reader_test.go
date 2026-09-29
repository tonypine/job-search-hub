package jobalerts_test

import (
	"context"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/jobalerts"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

type fakeGmail map[string]google.Message

func (gmail fakeGmail) GetMessage(_ context.Context, id string) (google.Message, error) {
	message, found := gmail[id]
	if !found {
		return google.Message{}, google.ErrMessageNotFound
	}
	return message, nil
}

func TestAlertsAddTheirJobsOnceAndSkipOnesTheHubAlreadyLists(t *testing.T) {
	database := testdatabase.New(t)
	hub := store.New(database)
	ctx := context.Background()
	recent := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	if _, err := hub.SyncFeedJobs(ctx, store.Actor{Kind: store.ActorSystem}, store.JobSourceHimalayas, []store.JobPosting{{
		ExternalID: "feed-1", CompanyName: "Globex, Inc.", Title: "Senior Frontend Engineer", URL: "https://example.com/jobs/1",
	}}, recent); err != nil {
		t.Fatalf("feed: %v", err)
	}
	alertText := "Senior frontend engineer\nGlobex\nSão Paulo\nView job: https://www.linkedin.com/comm/jobs/view/111/\n\n" +
		"Staff Engineer\nInitrode\nRemote\nView job: https://www.linkedin.com/comm/jobs/view/222/\n"
	record := func(gmailID, sender string, sentAt time.Time) {
		message, _, err := hub.RecordMailMessage(ctx, store.NewMailMessage{
			GmailMessageID: gmailID, ThreadID: gmailID, Direction: store.MailReceived, Sender: sender, Subject: "Jobs for you", SentAt: sentAt,
		})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if err := hub.SaveMailClassification(ctx, message.ID, store.MailClassification{Class: store.MailJobAlert, ClassifiedBy: store.ClassifiedByRule}); err != nil {
			t.Fatalf("classify: %v", err)
		}
	}
	record("alert-1", "LinkedIn <jobs-noreply@linkedin.com>", recent)
	record("alert-2", "LinkedIn <jobs-noreply@linkedin.com>", recent.Add(time.Minute))
	record("old-alert", "LinkedIn <jobs-noreply@linkedin.com>", recent.Add(-60*24*time.Hour))
	record("gone", "LinkedIn <jobs-noreply@linkedin.com>", recent)
	record("shop", "Shop <promotions@example.com>", recent)
	gmail := fakeGmail{
		"alert-1":   {Text: alertText},
		"alert-2":   {Text: alertText},
		"old-alert": {Text: alertText},
		"shop":      {Text: "Sale"},
	}
	reader := jobalerts.NewReader(hub, gmail)

	summary, err := reader.ReadOnce(ctx)

	if err != nil || summary != (jobalerts.PassSummary{Alerts: 4, Added: 1, AlreadyListed: 2}) {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	var count int
	var title, company string
	if err := database.QueryRow(ctx, `SELECT count(*) OVER (), title, company_name FROM jobs WHERE source = $1`, store.JobSourceLinkedIn).
		Scan(&count, &title, &company); err != nil || count != 1 || title != "Staff Engineer" || company != "Initrode" {
		t.Fatalf("linkedin jobs: %d %q %q, %v", count, title, company, err)
	}
	if again, err := reader.ReadOnce(ctx); err != nil || again != (jobalerts.PassSummary{}) {
		t.Errorf("a second pass = %+v, %v; want nothing left to read", again, err)
	}
}
