package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/linkedinexport"
	"github.com/tonypine/job-search-hub/server/internal/prompts"
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
// LinkedIn profile: importing the export's profile files at once, reading the
// profile with how it differs from the job criteria, and the prompt of an
// audit for the recruiters who search.
func RegisterLinkedInProfileRoutes(routes *http.ServeMux, hub *store.Store, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
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

	handle("GET /v1/linkedin/profile/audit-prompt", func(w http.ResponseWriter, r *http.Request) {
		saved, err := hub.GetJobCriteria(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		market, err := getMarket(r.Context(), hub, rateSource)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		recruiterHistory, err := getRecruiterHistory(r.Context(), hub)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		rendered, err := prompts.RenderProfileAudit(r.Context(), hub, saved.Criteria, market, recruiterHistory)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, replyPromptResponse{Prompt: rendered.Body, Version: rendered.Version})
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

// marketTopCount is how many titles and technologies the audit sees.
const marketTopCount = 15

// market is what the open postings that fit ask for: their most common
// titles and technologies, with how many postings name each.
type market struct {
	FittingPostings int          `json:"fitting_postings"`
	Titles          []namedCount `json:"titles"`
	Technologies    []namedCount `json:"technologies"`
}

type namedCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// getMarket counts the titles and technologies of the open jobs judged a
// good or unclear fit.
func getMarket(ctx context.Context, hub *store.Store, rateSource exchangeRateSource) (market, error) {
	criteria, rates, err := jobfit.ReadInputs(ctx, hub, rateSource)
	if err != nil {
		return market{}, err
	}
	titles, technologies := map[string]int{}, map[string]int{}
	var result market
	for offset := 0; ; offset += openJobsPageSize {
		jobs, total, err := hub.ListJobs(ctx, store.JobFilter{Status: store.JobStatusOpen, Limit: openJobsPageSize, Offset: offset})
		if err != nil {
			return market{}, err
		}
		for _, item := range jobs {
			if jobfit.Judge(item.Job, item.Facts, criteria, rates).Level == jobfit.LevelPoor {
				continue
			}
			result.FittingPostings++
			titles[item.Job.Title]++
			var facts struct {
				Technologies []string `json:"technologies"`
			}
			if json.Unmarshal(item.Facts, &facts) == nil {
				for _, technology := range facts.Technologies {
					technologies[technology]++
				}
			}
		}
		if offset+len(jobs) >= total || len(jobs) == 0 {
			break
		}
	}
	result.Titles, result.Technologies = getTopCounts(titles), getTopCounts(technologies)
	return result, nil
}

// getRecruiterHistory counts the roles and companies recruiters approached
// the owner for on LinkedIn: how recruiters read the profile.
func getRecruiterHistory(ctx context.Context, hub *store.Store) (map[string]any, error) {
	conversations, err := hub.ListRecruiterConversations(ctx)
	if err != nil {
		return nil, err
	}
	roles, companies := map[string]int{}, map[string]int{}
	for _, conversation := range conversations {
		if conversation.Role != "" {
			roles[conversation.Role]++
		}
		if conversation.HiringCompany != "" {
			companies[conversation.HiringCompany]++
		}
	}
	return map[string]any{"conversations": len(conversations), "roles": getTopCounts(roles), "companies": getTopCounts(companies)}, nil
}

func getTopCounts(counts map[string]int) []namedCount {
	named := make([]namedCount, 0, len(counts))
	for name, count := range counts {
		named = append(named, namedCount{Name: name, Count: count})
	}
	sort.Slice(named, func(a, b int) bool {
		if named[a].Count != named[b].Count {
			return named[a].Count > named[b].Count
		}
		return named[a].Name < named[b].Name
	})
	if len(named) > marketTopCount {
		named = named[:marketTopCount]
	}
	return named
}
