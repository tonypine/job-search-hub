package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/hubevents"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const heartbeatInterval = 25 * time.Second

// RegisterEventRoutes adds the owner-only event stream: server-sent events,
// one "update" event per recorded update, with the update's sequence as its
// ID. A reconnect sending Last-Event-ID first gets what it missed.
func RegisterEventRoutes(routes *http.ServeMux, hub *store.Store, broadcaster *hubevents.Broadcaster, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/events", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, canFlush := w.(http.Flusher)
		if !canFlush {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "the server can't stream"})
			return
		}
		updates, stop := broadcaster.Subscribe()
		defer stop()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		lastSent := int64(0)
		if lastEventID, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); err == nil {
			missed, err := hub.ListUpdatesAfter(r.Context(), lastEventID)
			if err != nil {
				return
			}
			for _, update := range missed {
				writeUpdateEvent(w, update)
				lastSent = update.Sequence
			}
			flusher.Flush()
		}

		heartbeat := time.NewTicker(heartbeatInterval)
		defer heartbeat.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case update, open := <-updates:
				if !open {
					return
				}
				if update.Sequence <= lastSent {
					continue
				}
				writeUpdateEvent(w, update)
				lastSent = update.Sequence
				flusher.Flush()
			case <-heartbeat.C:
				fmt.Fprint(w, ": heartbeat\n\n")
				flusher.Flush()
			}
		}
	})))
}

func writeUpdateEvent(w http.ResponseWriter, update store.Update) {
	data, err := json.Marshal(update)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "id: %d\nevent: update\ndata: %s\n\n", update.Sequence, data)
}
