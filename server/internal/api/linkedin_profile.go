package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/linkedinexport"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

type linkedInProfileResponse struct {
	Profile store.LinkedInProfile `json:"profile"`
	// CriteriaDifferences are where what LinkedIn tells recruiters differs
	// from what the hub looks for, for the owner to settle.
	CriteriaDifferences []string `json:"criteria_differences"`
}

type profileImportRequest struct {
	Files map[string]string `json:"files"`
}

type profileImportResponse struct {
	Read []string `json:"read"`
}

// RegisterLinkedInProfileRoutes adds the owner-only routes for the owner's
// LinkedIn profile: importing the export's profile files at once, and reading
// the profile with how it differs from the job criteria.
func RegisterLinkedInProfileRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	handle := func(pattern string, handler http.HandlerFunc) { routes.Handle(pattern, requireOwner(handler)) }

	handle("GET /v1/linkedin/profile", func(w http.ResponseWriter, r *http.Request) {
		profile, err := hub.GetLinkedInProfile(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		criteria, err := hub.GetJobCriteria(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, linkedInProfileResponse{Profile: profile, CriteriaDifferences: getCriteriaDifferences(profile, criteria.Criteria)})
	})

	handle("POST /v1/linkedin/profile/import", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maximumArchiveFileBytes)
		var request profileImportRequest
		if !decodeBodyOrWriteBadRequest(w, r, &request) {
			return
		}
		profile, err := hub.GetLinkedInProfile(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		read, err := linkedinexport.ApplyProfileFiles(&profile, request.Files)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		if len(read) == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "no profile file of a LinkedIn export was given"})
			return
		}
		if err := hub.SaveLinkedInProfile(r.Context(), store.Actor{Kind: store.ActorOwner}, profile); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, profileImportResponse{Read: read})
	})
}

// getCriteriaDifferences compares what the owner tells recruiters on LinkedIn
// with what the hub looks for: titles on one side only, recruiters not let in,
// and no locations where the hub counts some.
func getCriteriaDifferences(profile store.LinkedInProfile, criteria store.JobCriteria) []string {
	differences := []string{}
	preferences := profile.Preferences
	if preferences == nil {
		return differences
	}
	isListed := func(title string, list []string) bool {
		normalized := wordmatch.Normalize(title)
		return slices.ContainsFunc(list, func(listed string) bool { return wordmatch.Normalize(listed) == normalized })
	}
	var onlyOnLinkedIn, onlyInHub []string
	for _, title := range preferences.JobTitles {
		if !isListed(title, criteria.Roles) {
			onlyOnLinkedIn = append(onlyOnLinkedIn, title)
		}
	}
	for _, role := range criteria.Roles {
		if !isListed(role, preferences.JobTitles) {
			onlyInHub = append(onlyInHub, role)
		}
	}
	if len(onlyInHub) > 0 {
		differences = append(differences, fmt.Sprintf(
			"The hub looks for %s, but your LinkedIn job-seeker preferences don't list them, so recruiters searching for them may not find you.",
			quoteAll(onlyInHub)))
	}
	if len(onlyOnLinkedIn) > 0 {
		differences = append(differences, fmt.Sprintf("LinkedIn tells recruiters you want %s, which the hub's roles don't include.", quoteAll(onlyOnLinkedIn)))
	}
	if !strings.EqualFold(preferences.OpenToRecruiters, "yes") {
		differences = append(differences, "Open to recruiters is off on LinkedIn, so recruiters don't see that you're looking.")
	}
	if len(preferences.Locations) == 0 && len(criteria.EligibleLocationTerms) > 0 {
		differences = append(differences, fmt.Sprintf(
			"LinkedIn lists no locations you're open to, while the hub counts %s as places you can work from.", quoteAll(criteria.EligibleLocationTerms)))
	}
	return differences
}

func quoteAll(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = `"` + value + `"`
	}
	return strings.Join(quoted, ", ")
}
