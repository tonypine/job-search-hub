package api

import (
	"net/http"
	"sort"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// Where a suggestion came from.
const (
	suggestionSourceLinkedIn        = "linkedin"
	suggestionSourceStartupsGallery = "startups_gallery"
)

// companySuggestion is a company the hub doesn't hold, from the owner's
// LinkedIn follows or startups.gallery's remote list, with what makes it
// worth researching.
type companySuggestion struct {
	store.CompanyFollow
	Source      string `json:"source"`
	Website     string `json:"website,omitempty"`
	CareersURL  string `json:"careers_url,omitempty"`
	OpenJobs    int    `json:"open_jobs"`
	FittingJobs int    `json:"fitting_jobs"`
}

type companySuggestionsResponse struct {
	Suggestions []companySuggestion `json:"suggestions"`
}

// RegisterCompanySuggestionRoutes adds the owner-only list of companies to
// consider for the watch list: the ones the owner follows on LinkedIn, and
// the ones on startups.gallery's remote list whose board has a fitting job
// open, ranked by the fitting jobs they have open in the feed, then all
// their open jobs, then the owner's connections there.
func RegisterCompanySuggestionRoutes(routes *http.ServeMux, hub *store.Store, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/company-suggestions", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		follows, err := hub.ListCompanyFollowsNotInHub(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		galleryCompanies, err := hub.ListGalleryCompaniesNotInHub(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		byCompany := map[string]companyOpenings{}
		byBoard := map[uuid.UUID]companyOpenings{}
		if err := walkOpenJobs(r.Context(), hub, rateSource, func(item store.JobListItem, level jobfit.Level) {
			if item.CompanyName != nil {
				key := store.NormalizeCompanyName(*item.CompanyName)
				byCompany[key] = countOpening(byCompany[key], item, level)
			}
			if item.Job.JobBoardID != nil {
				byBoard[*item.Job.JobBoardID] = countOpening(byBoard[*item.Job.JobBoardID], item, level)
			}
		}); err != nil {
			writeStoreError(w, err)
			return
		}
		suggestions := make([]companySuggestion, 0, len(follows)+len(galleryCompanies))
		followed := map[string]bool{}
		for _, follow := range follows {
			key := store.NormalizeCompanyName(follow.Organization)
			followed[key] = true
			found := byCompany[key]
			suggestions = append(suggestions, companySuggestion{CompanyFollow: follow, Source: suggestionSourceLinkedIn, OpenJobs: found.open, FittingJobs: found.fitting})
		}
		for _, company := range galleryCompanies {
			found := byBoard[*company.JobBoardID]
			if found.fitting == 0 || followed[store.NormalizeCompanyName(company.Name)] {
				continue
			}
			suggestions = append(suggestions, companySuggestion{
				CompanyFollow: store.CompanyFollow{Organization: company.Name}, Source: suggestionSourceStartupsGallery,
				Website: company.Website, CareersURL: company.CareersURL, OpenJobs: found.open, FittingJobs: found.fitting,
			})
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
