package api

import (
	"errors"
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/linkedinexport"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// maximumConnectionsFileBytes is far above any LinkedIn network's export.
const maximumConnectionsFileBytes = 20 << 20

type connectionsImportResponse struct {
	store.ConnectionImport
	Skipped int `json:"skipped"`
}

// RegisterConnectionRoutes adds the owner-only routes for the owner's
// LinkedIn network: importing the export's Connections.csv, and a summary.
func RegisterConnectionRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	handle := func(pattern string, handler http.HandlerFunc) { routes.Handle(pattern, requireOwner(handler)) }

	handle("GET /v1/connections", func(w http.ResponseWriter, r *http.Request) {
		summary, err := hub.GetConnectionsSummary(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	})

	handle("POST /v1/connections/import", func(w http.ResponseWriter, r *http.Request) {
		connections, skipped, err := linkedinexport.ParseConnections(http.MaxBytesReader(w, r.Body, maximumConnectionsFileBytes))
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "the file is larger than 20 MB"})
			return
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		imported, err := hub.ImportConnections(r.Context(), store.Actor{Kind: store.ActorOwner}, connections)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, connectionsImportResponse{ConnectionImport: imported, Skipped: skipped})
	})
}
