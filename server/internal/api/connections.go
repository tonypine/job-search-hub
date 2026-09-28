package api

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/linkedinexport"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// maximumArchiveFileBytes is far above any file of a LinkedIn export,
// a long history of messages included.
const maximumArchiveFileBytes = 100 << 20

type connectionsImportResponse struct {
	store.ConnectionImport
	Skipped int `json:"skipped"`
}

// RegisterConnectionRoutes adds the owner-only routes for the owner's
// LinkedIn network: importing the export's Connections.csv, messages.csv and
// Invitations.csv, and a summary.
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
		connections, skipped, err := linkedinexport.ParseConnections(http.MaxBytesReader(w, r.Body, maximumArchiveFileBytes))
		if !writeImportReadError(w, err) {
			return
		}
		imported, err := hub.ImportConnections(r.Context(), store.Actor{Kind: store.ActorOwner}, connections)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, connectionsImportResponse{ConnectionImport: imported, Skipped: skipped})
	})

	handle("POST /v1/linkedin/messages/import", func(w http.ResponseWriter, r *http.Request) {
		messages, err := linkedinexport.ParseMessages(http.MaxBytesReader(w, r.Body, maximumArchiveFileBytes))
		if !writeImportReadError(w, err) {
			return
		}
		imported, err := hub.ImportLinkedInMessages(r.Context(), store.Actor{Kind: store.ActorOwner}, messages)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, imported)
	})

	handle("POST /v1/linkedin/invitations/import", func(w http.ResponseWriter, r *http.Request) {
		invitations, err := linkedinexport.ParseInvitations(http.MaxBytesReader(w, r.Body, maximumArchiveFileBytes))
		if !writeImportReadError(w, err) {
			return
		}
		imported, err := hub.ImportLinkedInInvitations(r.Context(), store.Actor{Kind: store.ActorOwner}, invitations)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, imported)
	})

	importLinkedInJobs := func(parse func(io.Reader) ([]store.NewLinkedInJob, int, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			jobs, unreadable, err := parse(http.MaxBytesReader(w, r.Body, maximumArchiveFileBytes))
			if !writeImportReadError(w, err) {
				return
			}
			imported, err := hub.ImportLinkedInJobs(r.Context(), store.Actor{Kind: store.ActorOwner}, jobs, time.Now())
			if err != nil {
				writeStoreError(w, err)
				return
			}
			imported.Skipped += unreadable
			writeJSON(w, http.StatusOK, imported)
		}
	}
	handle("POST /v1/linkedin/applications/import", importLinkedInJobs(linkedinexport.ParseJobApplications))
	handle("POST /v1/linkedin/saved-jobs/import", importLinkedInJobs(linkedinexport.ParseSavedJobs))
}

// writeImportReadError answers a file that couldn't be read, and reports
// whether the import can go on.
func writeImportReadError(w http.ResponseWriter, err error) bool {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "the file is larger than 100 MB"})
		return false
	case err != nil:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return false
	}
	return true
}
