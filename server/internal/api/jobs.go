package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type postingSource interface {
	FetchPosting(ctx context.Context, reference jobboards.PostingReference) (store.JobPosting, error)
}

type jobsResponse struct {
	Jobs  []judgedJobListItem `json:"jobs"`
	Total int                 `json:"total"`
}

// judgedJobListItem is a row of the jobs list with its fit.
type judgedJobListItem struct {
	store.JobListItem
	Fit jobfit.Fit `json:"fit"`
}

// judgedJobDetails are a job's details with its fit.
type judgedJobDetails struct {
	store.JobDetails
	Fit jobfit.Fit `json:"fit"`
}

type addJobRequest struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type addJobResponse struct {
	Job     store.Job `json:"job"`
	Created bool      `json:"created"`
}

// RegisterJobRoutes adds the owner-only routes for listing jobs, reading one
// job's details, and adding one by URL.
func RegisterJobRoutes(routes *http.ServeMux, hub *store.Store, postings postingSource, requireOwner func(http.Handler) http.Handler) {
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
		criteria, err := hub.GetJobCriteria(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		judged := make([]judgedJobListItem, 0, len(jobs))
		for _, item := range jobs {
			judged = append(judged, judgedJobListItem{JobListItem: item, Fit: jobfit.Judge(item.Job, item.Facts, criteria.Criteria)})
		}
		writeJSON(w, http.StatusOK, jobsResponse{Jobs: judged, Total: total})
	})))

	routes.Handle("GET /v1/jobs/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		details, err := hub.GetJobDetails(r.Context(), id)
		if errors.Is(err, store.ErrJobNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		criteria, err := hub.GetJobCriteria(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, judgedJobDetails{JobDetails: details, Fit: jobfit.Judge(details.Job, details.RawFacts, criteria.Criteria)})
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
