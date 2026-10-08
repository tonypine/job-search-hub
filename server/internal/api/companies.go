package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type companiesResponse struct {
	Companies []companyListItem `json:"companies"`
}

// companyListItem is a row of the companies list with what the company has
// open for the owner: its open jobs that pass the screen, the best match
// their briefs found, and when the newest of them was first seen.
type companyListItem struct {
	store.CompanySummary
	FittingJobs            int        `json:"fitting_jobs"`
	BestMatch              *string    `json:"best_match,omitempty"`
	NewestFittingJobSeenAt *time.Time `json:"newest_fitting_job_seen_at,omitempty"`
}

// matchRanks orders a brief's match classes, the best highest.
var matchRanks = map[string]int{"mismatch": 1, "stretch": 2, "possible": 3, "strong": 4}

// companyMailListSize is how many of a company's latest messages its page
// shows.
const companyMailListSize = 20

// companyMailFoldedClasses are the classes a company's page leaves out of its
// latest mail, so newsletters and alert digests from its domain don't crowd
// out the threads that matter. It says how many it left out instead.
var companyMailFoldedClasses = []string{store.MailNoise, store.MailJobAlert}

// companyResponse is the dossier with the threads open at the company: its
// cards on the board and its latest mail. Only the owner's app gets them;
// agents read the dossier alone, so mail written by senders never reaches
// their prompts.
type companyResponse struct {
	store.CompanyDossier
	Applications    []store.CompanyApplication `json:"applications"`
	Mail            []store.MailMessage        `json:"mail"`
	FoldedMailCount int                        `json:"folded_mail_count"`
}

// RegisterCompanyRoutes adds the owner-only routes the app reads companies
// through.
func RegisterCompanyRoutes(routes *http.ServeMux, hub *store.Store, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/companies", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		companies, err := listCompanies(r.Context(), hub, rateSource)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, companiesResponse{Companies: companies})
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

// listCompanies reads every company's row, and judges every open job to
// count the ones that pass the screen at each company.
func listCompanies(ctx context.Context, hub *store.Store, rateSource exchangeRateSource) ([]companyListItem, error) {
	summaries, err := hub.ListCompanySummaries(ctx)
	if err != nil {
		return nil, err
	}
	fitting := map[uuid.UUID]companyListItem{}
	err = walkOpenJobs(ctx, hub, rateSource, func(item store.JobListItem, level jobfit.Level) {
		if level != jobfit.LevelGood || item.Job.CompanyID == nil {
			return
		}
		counted := fitting[*item.Job.CompanyID]
		counted.FittingJobs++
		if item.Match != nil && (counted.BestMatch == nil || matchRanks[*item.Match] > matchRanks[*counted.BestMatch]) {
			counted.BestMatch = item.Match
		}
		if seenAt := item.Job.FirstSeenAt; counted.NewestFittingJobSeenAt == nil || seenAt.After(*counted.NewestFittingJobSeenAt) {
			counted.NewestFittingJobSeenAt = &seenAt
		}
		fitting[*item.Job.CompanyID] = counted
	})
	if err != nil {
		return nil, err
	}
	companies := make([]companyListItem, 0, len(summaries))
	for _, summary := range summaries {
		item := fitting[summary.Company.ID]
		item.CompanySummary = summary
		companies = append(companies, item)
	}
	return companies, nil
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
	mail, err := hub.ListMailMessages(ctx, store.MailFilter{CompanyID: &id, LeaveOutClasses: companyMailFoldedClasses, Limit: companyMailListSize})
	if err != nil {
		return companyResponse{}, err
	}
	folded, err := hub.CountCompanyMailOfClasses(ctx, id, companyMailFoldedClasses)
	if err != nil {
		return companyResponse{}, err
	}
	return companyResponse{CompanyDossier: dossier, Applications: applications, Mail: mail, FoldedMailCount: folded}, nil
}
