package api

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// relatedPerson is someone who can get the owner in, with the open jobs at
// their company, and how many of those fit.
type relatedPerson struct {
	store.RelatedPerson
	OpenJobs    int `json:"open_jobs"`
	FittingJobs int `json:"fitting_jobs"`
}

type peopleResponse struct {
	People []relatedPerson `json:"people"`
}

// RegisterPeopleRoutes adds the owner-only list of everyone who can get the
// owner in: contacts, connections, introducers and recruiters, each flagged
// by what their company has open now. company_id narrows it to one company.
func RegisterPeopleRoutes(routes *http.ServeMux, hub *store.Store, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/people", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var companyID *uuid.UUID
		if raw := r.URL.Query().Get("company_id"); raw != "" {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "company_id must be a company's id"})
				return
			}
			companyID = &parsed
		}
		listed, err := hub.ListRelatedPeople(r.Context(), companyID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		openings, err := getOpeningsByCompany(r.Context(), hub, rateSource)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		people := make([]relatedPerson, 0, len(listed))
		for _, person := range listed {
			shown := relatedPerson{RelatedPerson: person}
			if found, ok := openings[store.NormalizeCompanyName(person.CompanyName)]; ok && person.CompanyName != "" {
				shown.OpenJobs, shown.FittingJobs = found.open, found.fitting
			}
			people = append(people, shown)
		}
		writeJSON(w, http.StatusOK, peopleResponse{People: people})
	})))
}
