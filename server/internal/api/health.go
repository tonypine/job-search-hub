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
	Database string          `json:"database"`
	Postgres *PostgresHealth `json:"postgres,omitempty"`
}

// PostgresHealth is the Postgres the server runs itself.
type PostgresHealth struct {
	// Major is the major of the cluster it runs.
	Major int `json:"major"`
	// UpgradeFailedTo is the newer major the server failed to move the
	// database to at start, so it runs the older one; 0 when none failed.
	UpgradeFailedTo int `json:"upgrade_failed_to,omitempty"`
}

// NewHealthHandler answers 200 when the database answers a ping, and 503 when
// it does not. With postgres, a database the server runs itself, it also says
// which major it is on.
func NewHealthHandler(database databasePinger, postgres *PostgresHealth) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), databasePingTimeout)
		defer cancel()

		if err := database.Ping(ctx); err != nil {
			slog.Warn("database ping failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Database: "unreachable", Postgres: postgres})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Database: "ok", Postgres: postgres})
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("writing response failed", "error", err)
	}
}
