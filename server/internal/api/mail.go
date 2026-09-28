package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/gmailwatch"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	defaultMailListSize = 100
	maximumMailListSize = 1000
	defaultBackfillDays = 60
	maximumBackfillDays = 365
)

// MailBackfiller records and matches past mail; it is nil when the hub
// doesn't hear Gmail's changes.
type MailBackfiller interface {
	Backfill(ctx context.Context, days int) (gmailwatch.BackfillResult, error)
}

type mailListResponse struct {
	Messages []store.MailMessage `json:"messages"`
}

type backfillRequest struct {
	Days int `json:"days"`
}

// RegisterMailRoutes adds the owner-only routes for recorded mail: listing
// it, all or one company's or the unmatched, and backfilling past days.
func RegisterMailRoutes(routes *http.ServeMux, hub *store.Store, backfiller MailBackfiller, requireOwner func(http.Handler) http.Handler) {
	handle := func(pattern string, handler http.HandlerFunc) { routes.Handle(pattern, requireOwner(handler)) }

	handle("GET /v1/mail", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		filter := store.MailFilter{UnmatchedOnly: query.Get("unmatched") == "true"}
		filter.Limit, _ = strconv.Atoi(query.Get("limit"))
		if filter.Limit <= 0 || filter.Limit > maximumMailListSize {
			filter.Limit = defaultMailListSize
		}
		if raw := query.Get("company_id"); raw != "" {
			companyID, err := uuid.Parse(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "company_id must be a UUID"})
				return
			}
			filter.CompanyID = &companyID
		}
		messages, err := hub.ListMailMessages(r.Context(), filter)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mailListResponse{Messages: messages})
	})

	handle("POST /v1/mail/backfill", func(w http.ResponseWriter, r *http.Request) {
		if backfiller == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "the hub doesn't hear Gmail's changes: set HUB_GMAIL_PUBSUB_TOPIC and HUB_GMAIL_PUBSUB_SUBSCRIPTION"})
			return
		}
		request := backfillRequest{Days: defaultBackfillDays}
		if r.ContentLength != 0 && !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		if request.Days <= 0 || request.Days > maximumBackfillDays {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "days must be between 1 and 365"})
			return
		}
		result, err := backfiller.Backfill(r.Context(), request.Days)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}
