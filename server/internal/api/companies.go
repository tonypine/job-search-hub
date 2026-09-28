package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type companiesResponse struct {
	Companies []store.CompanySummary `json:"companies"`
}

// RegisterCompanyRoutes adds the owner-only routes the app reads companies
// through.
func RegisterCompanyRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/companies", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		summaries, err := hub.ListCompanySummaries(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, companiesResponse{Companies: summaries})
	})))

	routes.Handle("GET /v1/companies/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: store.ErrCompanyNotFound.Error()})
			return
		}
		dossier, err := hub.GetCompanyDossier(r.Context(), id)
		switch {
		case errors.Is(err, store.ErrCompanyNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, dossier)
		}
	})))
}
