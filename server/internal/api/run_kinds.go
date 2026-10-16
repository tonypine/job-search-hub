package api

import (
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type runKindsResponse struct {
	Groups []store.RunGroup `json:"groups"`
	Kinds  []store.RunKind  `json:"kinds"`
}

// RegisterRunKindRoutes adds the owner-only route that lists every kind of
// run the hub records, by group.
func RegisterRunKindRoutes(routes *http.ServeMux, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/run-kinds", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, runKindsResponse{Groups: store.RunGroups, Kinds: store.RunKinds})
	})))
}
