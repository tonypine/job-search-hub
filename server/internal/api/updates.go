package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/hubevents"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	defaultUpdateListSize = 100
	maximumUpdateListSize = 500
)

type recordUpdateRequest struct {
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	JobID     *uuid.UUID `json:"job_id"`
	CompanyID *uuid.UUID `json:"company_id"`
	SourceURL string     `json:"source_url"`
}

type markedSeenResponse struct {
	Marked int `json:"marked"`
}

// RegisterUpdateRoutes adds the owner-only routes for the log of updates:
// listing them, recording one by hand, and marking them seen.
func RegisterUpdateRoutes(routes *http.ServeMux, hub *store.Store, recorder *hubevents.Recorder, requireOwner func(http.Handler) http.Handler) {
	handle := func(pattern string, handler http.HandlerFunc) { routes.Handle(pattern, requireOwner(handler)) }

	handle("GET /v1/updates", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > maximumUpdateListSize {
			limit = defaultUpdateListSize
		}
		list, err := hub.ListUpdates(r.Context(), limit, r.URL.Query().Get("unseen") == "true")
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})

	handle("POST /v1/updates", func(w http.ResponseWriter, r *http.Request) {
		var request recordUpdateRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		update, err := recorder.Record(r.Context(), store.NewUpdate(request))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, update)
	})

	handle("POST /v1/updates/seen", func(w http.ResponseWriter, r *http.Request) {
		var selection store.UpdateSelection
		if !decodeBodyOrWriteBadRequest(w, r, &selection) {
			return
		}
		marked, err := hub.MarkUpdatesSeen(r.Context(), selection)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, markedSeenResponse{Marked: marked})
	})
}
