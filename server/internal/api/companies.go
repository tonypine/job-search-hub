package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type companiesResponse struct {
	Companies []store.CompanySummary `json:"companies"`
}

// companyMailListSize is how many of a company's latest messages its page
// shows.
const companyMailListSize = 20

// companyResponse is the dossier with the threads open at the company: its
// cards on the board and its latest mail. Only the owner's app gets them;
// agents read the dossier alone, so mail written by senders never reaches
// their prompts.
type companyResponse struct {
	store.CompanyDossier
	Applications []store.CompanyApplication `json:"applications"`
	Mail         []store.MailMessage        `json:"mail"`
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
		response, err := getCompanyResponse(r.Context(), hub, id)
		switch {
		case errors.Is(err, store.ErrCompanyNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, response)
		}
	})))
}

func getCompanyResponse(ctx context.Context, hub *store.Store, id uuid.UUID) (companyResponse, error) {
	dossier, err := hub.GetCompanyDossier(ctx, id)
	if err != nil {
		return companyResponse{}, err
	}
	applications, err := hub.ListCompanyApplications(ctx, id)
	if err != nil {
		return companyResponse{}, err
	}
	mail, err := hub.ListMailMessages(ctx, store.MailFilter{CompanyID: &id, Limit: companyMailListSize})
	if err != nil {
		return companyResponse{}, err
	}
	return companyResponse{CompanyDossier: dossier, Applications: applications, Mail: mail}, nil
}
