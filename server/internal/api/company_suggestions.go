package api

import (
	"net/http"
	"sort"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// companySuggestion is a company the owner follows on LinkedIn but the hub
// doesn't hold, with what makes it worth researching.
type companySuggestion struct {
	store.CompanyFollow
	OpenJobs    int `json:"open_jobs"`
	FittingJobs int `json:"fitting_jobs"`
}

type companySuggestionsResponse struct {
	Suggestions []companySuggestion `json:"suggestions"`
}

// RegisterCompanySuggestionRoutes adds the owner-only list of companies to
// consider for the watch list: the ones the owner follows on LinkedIn, ranked
// by the fitting jobs they have open in the feed, then all their open jobs,
// then the owner's connections there.
func RegisterCompanySuggestionRoutes(routes *http.ServeMux, hub *store.Store, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/company-suggestions", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		follows, err := hub.ListCompanyFollowsNotInHub(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		openings, err := getOpeningsByCompany(r.Context(), hub, rateSource)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		suggestions := make([]companySuggestion, 0, len(follows))
		for _, follow := range follows {
			found := openings[store.NormalizeCompanyName(follow.Organization)]
			suggestions = append(suggestions, companySuggestion{CompanyFollow: follow, OpenJobs: found.open, FittingJobs: found.fitting})
		}
		sort.SliceStable(suggestions, func(a, b int) bool {
			first, second := suggestions[a], suggestions[b]
			if first.FittingJobs != second.FittingJobs {
				return first.FittingJobs > second.FittingJobs
			}
			if first.OpenJobs != second.OpenJobs {
				return first.OpenJobs > second.OpenJobs
			}
			return first.ConnectionCount > second.ConnectionCount
		})
		writeJSON(w, http.StatusOK, companySuggestionsResponse{Suggestions: suggestions})
	})))
}
