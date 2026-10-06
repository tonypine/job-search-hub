package jobalerts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumAlertsPerPass bounds one pass; the next picks up the rest.
	maximumAlertsPerPass = 200
	// alertWindow is how old an alert can be and still be read: its postings
	// would already have expired.
	alertWindow = postingLifetime
)

// alertReader is who the change log says added the jobs; each job's source
// is its posting.
var alertReader = store.Actor{Kind: store.ActorSystem}

type messageFetcher interface {
	GetMessage(ctx context.Context, id string) (google.Message, error)
}

type Reader struct {
	hub    *store.Store
	gmail  messageFetcher
	nudges chan struct{}
	now    func() time.Time
}

func NewReader(hub *store.Store, gmail messageFetcher) *Reader {
	return &Reader{hub: hub, gmail: gmail, nudges: make(chan struct{}, 1), now: time.Now}
}

// Nudge asks for a pass now, as when mail was classified.
func (reader *Reader) Nudge() {
	select {
	case reader.nudges <- struct{}{}:
	default:
	}
}

// Run reads at start, then every interval and on each nudge, until ctx ends.
func (reader *Reader) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			summary, err := reader.ReadOnce(ctx)
			if err != nil {
				slog.Error("job alert reading stopped", "error", err, "alerts", summary.Alerts)
			} else if summary.Alerts > 0 {
				slog.Info("job alerts read", "alerts", summary.Alerts, "added", summary.Added, "already listed", summary.AlreadyListed)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-reader.nudges:
		}
	}
}

// PassSummary counts one pass over the unread alerts.
type PassSummary struct {
	Alerts        int
	Added         int
	AlreadyListed int
}

// ReadOnce reads the jobs of every unread alert of the last alertWindow,
// oldest first, so a later alert's sighting wins.
func (reader *Reader) ReadOnce(ctx context.Context) (PassSummary, error) {
	var summary PassSummary
	alerts, err := reader.hub.ListUnreadJobAlerts(ctx, reader.now().Add(-alertWindow), maximumAlertsPerPass)
	if err != nil {
		return summary, err
	}
	for _, message := range alerts {
		found, err := reader.readAlert(ctx, message, &summary)
		if err != nil {
			return summary, fmt.Errorf("read the jobs in %q: %w", message.Subject, err)
		}
		if err := reader.hub.MarkJobAlertRead(ctx, message.ID, found); err != nil {
			return summary, err
		}
		summary.Alerts++
	}
	return summary, nil
}

// readAlert stores the jobs one alert lists and returns how many it listed.
// Mail from a sender whose alerts the hub doesn't read, or gone from Gmail,
// lists none.
func (reader *Reader) readAlert(ctx context.Context, message store.MailMessage, summary *PassSummary) (int, error) {
	source, known := GetSource(message.Sender)
	if !known {
		return 0, nil
	}
	fetched, err := reader.gmail.GetMessage(ctx, message.GmailMessageID)
	if errors.Is(err, google.ErrMessageNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	postings := ParsePostings(source, Alert{Sender: message.Sender, Subject: message.Subject, SentAt: message.SentAt, Text: fetched.Text, HTML: fetched.HTML})
	if len(postings) == 0 {
		return 0, nil
	}
	result, alreadyListed, err := reader.hub.SyncAlertJobs(ctx, alertReader, source, postings, message.SentAt)
	if err != nil {
		return 0, err
	}
	summary.Added += result.Created
	summary.AlreadyListed += alreadyListed
	return len(postings), nil
}
