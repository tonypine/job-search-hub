package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type postingSource interface {
	FetchPosting(ctx context.Context, reference jobboards.PostingReference) (store.JobPosting, error)
}

type exchangeRateSource = jobfit.RateSource

// getJudgedJobDetails reads a job's details and judges its fit.
func getJudgedJobDetails(ctx context.Context, hub *store.Store, rateSource exchangeRateSource, id uuid.UUID) (judgedJobDetails, error) {
	details, err := hub.GetJobDetails(ctx, id)
	if err != nil {
		return judgedJobDetails{}, err
	}
	criteria, rates, err := jobfit.ReadInputs(ctx, hub, rateSource)
	if err != nil {
		return judgedJobDetails{}, err
	}
	confirmed := true
	roles, err := hub.ListProfileEntries(ctx, store.ProfileEntryFilter{Kind: store.ProfileEntryRole, Confirmed: &confirmed})
	if err != nil {
		return judgedJobDetails{}, err
	}
	fit := jobfit.Judge(details.Job, details.RawFacts, criteria, rates)
	screenOut := buildScreenOutAnswers(details, fit, countExperienceMonths(roles, time.Now()))
	return judgedJobDetails{JobDetails: details, Fit: fit, ScreenOut: screenOut}, nil
}

type jobsResponse struct {
	Jobs  []judgedJobListItem `json:"jobs"`
	Total int                 `json:"total"`
	// FactColumns are the facts the jobs' facts hold, as list columns.
	FactColumns []store.JobFactColumn `json:"fact_columns"`
}

// judgedJobListItem is a row of the jobs list with its fit.
type judgedJobListItem struct {
	store.JobListItem
	Fit jobfit.Fit `json:"fit"`
}

// judgedJobDetails are a job's details with its fit and the answers that
// could screen the owner out.
type judgedJobDetails struct {
	store.JobDetails
	Fit       jobfit.Fit        `json:"fit"`
	ScreenOut []screenOutAnswer `json:"screen_out"`
}

type addJobRequest struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type addJobResponse struct {
	Job     store.Job `json:"job"`
	Created bool      `json:"created"`
}

// jobDismissalRequest names the jobs to dismiss or restore; the reason is
// the owner's own words, and restoring ignores it.
type jobDismissalRequest struct {
	JobIDs []uuid.UUID `json:"job_ids"`
	Reason string      `json:"reason"`
}

type jobDismissalResponse struct {
	Jobs []store.Job `json:"jobs"`
}

// jobDecisionRequest is the owner's decision on a job: pursue, skip or
// later, with a reason a skip keeps.
type jobDecisionRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// RegisterJobRoutes adds the owner-only routes for listing jobs, reading one
// job's details, adding one by URL, dismissing and restoring jobs, and
// deciding on one.
func RegisterJobRoutes(routes *http.ServeMux, hub *store.Store, postings postingSource, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/jobs", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parameters := r.URL.Query()
		filter := store.JobFilter{Query: parameters.Get("query"), Status: parameters.Get("status")}
		if raw := parameters.Get("company_id"); raw != "" {
			companyID, err := uuid.Parse(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "company_id must be a uuid"})
				return
			}
			filter.CompanyID = &companyID
		}
		filter.Limit, _ = strconv.Atoi(parameters.Get("limit"))
		filter.Offset, _ = strconv.Atoi(parameters.Get("offset"))

		jobs, total, err := hub.ListJobs(r.Context(), filter)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		criteria, rates, err := jobfit.ReadInputs(r.Context(), hub, rateSource)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		judged := make([]judgedJobListItem, 0, len(jobs))
		for _, item := range jobs {
			judged = append(judged, judgedJobListItem{JobListItem: item, Fit: jobfit.Judge(item.Job, item.Facts, criteria, rates)})
		}
		factColumns, err := hub.ListJobFactColumns(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, jobsResponse{Jobs: judged, Total: total, FactColumns: factColumns})
	})))

	routes.Handle("GET /v1/jobs/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		details, err := getJudgedJobDetails(r.Context(), hub, rateSource, id)
		if errors.Is(err, store.ErrJobNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, details)
	})))

	routes.Handle("POST /v1/jobs", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request addJobRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		job, created, err := addJobByURL(r.Context(), hub, postings, request)
		switch {
		case errors.Is(err, store.ErrCompanyNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		case created:
			writeJSON(w, http.StatusCreated, addJobResponse{Job: job, Created: true})
		default:
			writeJSON(w, http.StatusOK, addJobResponse{Job: job})
		}
	})))

	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("POST /v1/jobs/dismiss", requireOwner(handleJobDismissal(func(ctx context.Context, request jobDismissalRequest) ([]store.Job, error) {
		return hub.DismissJobs(ctx, owner, request.JobIDs, strings.TrimSpace(request.Reason))
	})))
	routes.Handle("POST /v1/jobs/restore", requireOwner(handleJobDismissal(func(ctx context.Context, request jobDismissalRequest) ([]store.Job, error) {
		return hub.RestoreJobs(ctx, owner, request.JobIDs)
	})))

	routes.Handle("POST /v1/jobs/{id}/decision", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		var request jobDecisionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		decision, err := hub.DecideJob(r.Context(), owner, id, request.Decision, request.Reason)
		switch {
		case errors.Is(err, store.ErrJobNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, decision)
		}
	})))
	routes.Handle("DELETE /v1/jobs/{id}/decision", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		switch err := hub.ClearJobDecision(r.Context(), owner, id); {
		case errors.Is(err, store.ErrJobNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})))
}

// handleJobDismissal serves a dismissal or restore: every job it names
// changes, or none does.
func handleJobDismissal(apply func(context.Context, jobDismissalRequest) ([]store.Job, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request jobDismissalRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		jobs, err := apply(r.Context(), request)
		switch {
		case errors.Is(err, store.ErrJobNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "a job named isn't in the hub"})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, jobDismissalResponse{Jobs: jobs})
		}
	})
}

// addJobByURL stores the job behind a URL. A Greenhouse, Lever or Ashby
// posting is read from its provider; when its board is already stored, the
// job joins that board's jobs, so the board poller never duplicates it. Any
// other URL, or a posting that cannot be read, is stored as given.
func addJobByURL(ctx context.Context, hub *store.Store, postings postingSource, request addJobRequest) (store.Job, bool, error) {
	owner := store.Actor{Kind: store.ActorOwner}
	manual := store.ManualJobInput{Title: request.Title, URL: request.URL}

	reference, isPostingURL := jobboards.ParsePostingURL(request.URL)
	if !isPostingURL {
		return hub.AddManualJob(ctx, owner, manual)
	}
	posting, err := postings.FetchPosting(ctx, reference)
	if err != nil {
		if manual.Title == "" {
			return store.Job{}, false, errors.New("the posting could not be read (" + err.Error() + "); give a title to store it as given")
		}
		return hub.AddManualJob(ctx, owner, manual)
	}

	board, err := hub.GetJobBoardByToken(ctx, reference.Provider, reference.BoardToken)
	if errors.Is(err, store.ErrJobBoardNotFound) {
		return hub.AddManualJob(ctx, owner, store.ManualJobInput{
			Title: posting.Title, URL: posting.URL, Location: posting.Location, Description: posting.Description,
		})
	}
	if err != nil {
		return store.Job{}, false, err
	}
	return hub.UpsertBoardJob(ctx, owner, board, posting, time.Now())
}
