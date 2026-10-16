package api

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

var runOutcomes = []string{store.RunOutcomeSucceeded, store.RunOutcomeFailed, store.RunOutcomeInvalid, store.RunOutcomeRunning}

// RegisterRunRoutes adds the owner-only route that lists the runs of both
// records, model calls and agent runs, the latest first, a page at a time.
func RegisterRunRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/runs", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter, err := readRunFilter(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		page, err := hub.ListRuns(r.Context(), filter)
		switch {
		case errors.Is(err, store.ErrInvalidRunCursor):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, page)
		}
	})))
}

// readRunFilter reads the list's query: kind, subject_type, subject_id,
// outcome, since and until (RFC 3339), cursor and limit.
func readRunFilter(r *http.Request) (store.RunFilter, error) {
	query := r.URL.Query()
	filter := store.RunFilter{
		Kind: query.Get("kind"), SubjectType: query.Get("subject_type"), Outcome: query.Get("outcome"), Cursor: query.Get("cursor"),
	}
	if _, found := store.GetRunKind(filter.Kind); filter.Kind != "" && !found {
		return filter, errors.New("no kind of run is called " + strconv.Quote(filter.Kind))
	}
	if filter.Outcome != "" && !slices.Contains(runOutcomes, filter.Outcome) {
		return filter, errors.New("an outcome is succeeded, failed, invalid or running")
	}
	if text := query.Get("subject_id"); text != "" {
		id, err := uuid.Parse(text)
		if err != nil {
			return filter, errors.New("subject_id must be an id")
		}
		filter.SubjectID = &id
	}
	for _, bound := range []struct {
		name string
		at   **time.Time
	}{{"since", &filter.Since}, {"until", &filter.Until}} {
		if text := query.Get(bound.name); text != "" {
			at, err := time.Parse(time.RFC3339, text)
			if err != nil {
				return filter, errors.New(bound.name + " must be an RFC 3339 time, such as 2026-10-16T09:00:00Z")
			}
			*bound.at = &at
		}
	}
	if text := query.Get("limit"); text != "" {
		limit, err := strconv.Atoi(text)
		if err != nil || limit <= 0 {
			return filter, errors.New("limit must be a positive number")
		}
		filter.Limit = limit
	}
	return filter, nil
}
