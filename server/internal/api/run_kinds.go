package api

import (
	"errors"
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type runKindsResponse struct {
	Groups []store.RunGroup         `json:"groups"`
	Kinds  []modelwork.RunKindState `json:"kinds"`
}

// RegisterRunKindRoutes adds the owner-only routes that list every kind of
// run the hub records, by group, with what each is doing now, and pause and
// resume the kinds the hub starts by itself, one at a time or all at once.
// Each change answers with the list.
func RegisterRunKindRoutes(routes *http.ServeMux, controls *modelwork.Controls, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	writeKinds := func(w http.ResponseWriter, r *http.Request) {
		kinds, err := controls.ListRunKinds(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, runKindsResponse{Groups: store.RunGroups, Kinds: kinds})
	}
	routes.Handle("GET /v1/run-kinds", requireOwner(http.HandlerFunc(writeKinds)))
	for action, paused := range map[string]bool{"pause": true, "resume": false} {
		routes.Handle("POST /v1/run-kinds/{kind}/"+action, requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := controls.SetPaused(r.Context(), owner, []string{r.PathValue("kind")}, paused)
			switch {
			case errors.Is(err, store.ErrRunKindNotFound):
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			case errors.Is(err, store.ErrRunKindNotPausable):
				writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
			case err != nil:
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			default:
				writeKinds(w, r)
			}
		})))
		routes.Handle("POST /v1/run-kinds/"+action+"-all", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := controls.SetAllPaused(r.Context(), owner, paused); err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			writeKinds(w, r)
		})))
	}
}
