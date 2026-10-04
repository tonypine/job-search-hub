package push

import (
	"context"
	"errors"
	"log/slog"

	"github.com/tonypine/job-search-hub/server/internal/hubevents"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// unpushedKinds are updates the phone already knows about, because it made
// them.
var unpushedKinds = map[string]bool{"task_queued": true}

type messageSender interface {
	Send(ctx context.Context, pushToken string, message Message) error
}

// Notifier pushes each update the hub records to every phone registered for
// pushes.
type Notifier struct {
	hub         *store.Store
	sender      messageSender
	broadcaster *hubevents.Broadcaster
}

func NewNotifier(hub *store.Store, sender messageSender, broadcaster *hubevents.Broadcaster) *Notifier {
	return &Notifier{hub: hub, sender: sender, broadcaster: broadcaster}
}

// Run pushes updates as they are recorded, until ctx ends. Pushes are best
// effort: an update recorded while the notifier lags too far behind is only
// on the phone's list.
func (notifier *Notifier) Run(ctx context.Context) {
	for ctx.Err() == nil {
		updates, stop := notifier.broadcaster.Subscribe()
		notifier.pushUntilDropped(ctx, updates)
		stop()
	}
}

func (notifier *Notifier) pushUntilDropped(ctx context.Context, updates <-chan store.Update) {
	for {
		select {
		case <-ctx.Done():
			return
		case update, open := <-updates:
			if !open {
				slog.Warn("push fell behind the hub's updates; some went unpushed")
				return
			}
			notifier.Push(ctx, update)
		}
	}
}

// Push sends one update to each registered phone, and forgets the tokens FCM
// says are gone.
func (notifier *Notifier) Push(ctx context.Context, update store.Update) {
	if unpushedKinds[update.Kind] {
		return
	}
	pushTokens, err := notifier.hub.ListDevicePushTokens(ctx)
	if err != nil {
		slog.Error("push: list the phones' tokens", "error", err)
		return
	}
	message := Message{UpdateID: update.ID.String(), Kind: update.Kind, Title: update.Title, Body: update.Body}
	if update.JobID != nil {
		message.JobID = update.JobID.String()
	}
	if update.CompanyID != nil {
		message.CompanyID = update.CompanyID.String()
	}
	for _, pushToken := range pushTokens {
		err := notifier.sender.Send(ctx, pushToken, message)
		switch {
		case errors.Is(err, ErrTokenGone):
			if err := notifier.hub.ForgetDevicePushToken(ctx, pushToken); err != nil {
				slog.Error("push: forget a gone token", "error", err)
			}
		case err != nil:
			slog.Warn("push failed", "update", update.ID, "error", err)
		}
	}
}
