// Package api holds the hub's REST handlers.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

const databasePingTimeout = 2 * time.Second

type databasePinger interface {
	Ping(ctx context.Context) error
}

type healthResponse struct {
	Database string `json:"database"`
}

// NewHealthHandler answers 200 when the database answers a ping, and 503 when
// it does not.
func NewHealthHandler(database databasePinger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), databasePingTimeout)
		defer cancel()

		if err := database.Ping(ctx); err != nil {
			slog.Warn("database ping failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Database: "unreachable"})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Database: "ok"})
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("writing response failed", "error", err)
	}
}
