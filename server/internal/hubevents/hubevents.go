// Package hubevents records updates and announces each one to the apps
// listening on the hub's event stream.
package hubevents

import (
	"context"
	"sync"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// subscriberBuffer is how many updates a slow listener may fall behind by
// before it is dropped; it reconnects and catches up from its last event.
const subscriberBuffer = 64

type Broadcaster struct {
	mutex       sync.Mutex
	subscribers map[chan store.Update]struct{}
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subscribers: map[chan store.Update]struct{}{}}
}

// Subscribe returns the updates announced from now on, and a function that
// stops them. A closed channel means the listener fell too far behind.
func (broadcaster *Broadcaster) Subscribe() (<-chan store.Update, func()) {
	channel := make(chan store.Update, subscriberBuffer)
	broadcaster.mutex.Lock()
	broadcaster.subscribers[channel] = struct{}{}
	broadcaster.mutex.Unlock()
	return channel, func() {
		broadcaster.mutex.Lock()
		defer broadcaster.mutex.Unlock()
		if _, subscribed := broadcaster.subscribers[channel]; subscribed {
			delete(broadcaster.subscribers, channel)
			close(channel)
		}
	}
}

func (broadcaster *Broadcaster) announce(update store.Update) {
	broadcaster.mutex.Lock()
	defer broadcaster.mutex.Unlock()
	for channel := range broadcaster.subscribers {
		select {
		case channel <- update:
		default:
			delete(broadcaster.subscribers, channel)
			close(channel)
		}
	}
}

// Recorder is the one way updates are recorded: stored, then announced.
type Recorder struct {
	hub         *store.Store
	broadcaster *Broadcaster
}

func NewRecorder(hub *store.Store, broadcaster *Broadcaster) *Recorder {
	return &Recorder{hub: hub, broadcaster: broadcaster}
}

func (recorder *Recorder) Record(ctx context.Context, input store.NewUpdate) (store.Update, error) {
	update, err := recorder.hub.RecordUpdate(ctx, input)
	if err != nil {
		return store.Update{}, err
	}
	recorder.broadcaster.announce(update)
	return update, nil
}
