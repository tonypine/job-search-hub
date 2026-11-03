package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/postingpages"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type postingSource interface {
	FetchPosting(ctx context.Context, reference jobboards.PostingReference) (store.JobPosting, error)
}

type pageSource interface {
	ReadPage(ctx context.Context, pageURL string) (postingpages.Page, error)
}

type jobRunStarter interface {
	StartJobRuns(ctx context.Context, actor store.Actor, jobID uuid.UUID)
}

// JobAdding is what adding a job by URL reads its posting with, and what
// starts its runs once it's added.
type JobAdding struct {
	// Postings reads a posting from its provider.
	Postings postingSource
	// Pages reads any other page; nil stores such a link as given.
	Pages pageSource
	// Runs starts a job's runs once it's added; nil starts none.
	Runs jobRunStarter
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
	experienceMonths, err := readExperienceMonths(ctx, hub)
	if err != nil {
		return judgedJobDetails{}, err
	}
	fit := jobfit.Judge(details.Job, details.RawFacts, criteria, rates)
	screenOut := buildScreenOutAnswers(indexLabelledFacts(details), fit, experienceMonths)
	return judgedJobDetails{JobDetails: details, Fit: fit, ScreenOut: screenOut}, nil
}

type jobsResponse struct {
	Jobs  []judgedJobListItem `json:"jobs"`
	Total int                 `json:"total"`
	// FactColumns are the facts the jobs' facts hold, as list columns.
	FactColumns []store.JobFactColumn `json:"fact_columns"`
}

// judgedJobListItem is a row of the jobs list with its fit and the answers
// that could screen the owner out, as its details have them, so the row's
// screen reads as the job's page does.
type judgedJobListItem struct {
	store.JobListItem
	Fit       jobfit.Fit        `json:"fit"`
	ScreenOut []screenOutAnswer `json:"screen_out"`
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
	// RunsStarted says the hub started the new job's runs, which its Run
	// menu lists running, or queued until the runs they read end.
	RunsStarted bool `json:"runs_started"`
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
// job's details, adding one by URL, which starts its runs, dismissing and
// restoring jobs, and deciding on one.
func RegisterJobRoutes(routes *http.ServeMux, hub *store.Store, adding JobAdding, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
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
		experienceMonths, err := readExperienceMonths(r.Context(), hub)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		judged := make([]judgedJobListItem, 0, len(jobs))
		for _, item := range jobs {
			fit := jobfit.Judge(item.Job, item.Facts, criteria, rates)
			screenOut := buildScreenOutAnswers(indexFlatFacts(item.Facts), fit, experienceMonths)
			judged = append(judged, judgedJobListItem{JobListItem: item, Fit: fit, ScreenOut: screenOut})
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
		job, created, err := addJobByURL(r.Context(), hub, adding, request)
		switch {
		case errors.Is(err, store.ErrCompanyNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		case created:
			response := addJobResponse{Job: job, Created: true}
			if adding.Runs != nil {
				adding.Runs.StartJobRuns(r.Context(), owner, job.ID)
				response.RunsStarted = true
			}
			writeJSON(w, http.StatusCreated, response)
		default:
			writeJSON(w, http.StatusOK, addJobResponse{Job: job})
		}
	})))

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
		cleared, err := hub.ClearJobDecision(r.Context(), owner, id)
		switch {
		case errors.Is(err, store.ErrJobNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, cleared)
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
// job joins that board's jobs, and when it isn't, the job names the posting,
// so the board's sync adopts it once the board is stored. Any other URL is
// read as a page: the provider posting it links to, else its JobPosting
// data, else its main content. Only a page that can't be read, or has no
// text, is stored as given, with the reason, and needs a title.
func addJobByURL(ctx context.Context, hub *store.Store, adding JobAdding, request addJobRequest) (store.Job, bool, error) {
	given := store.ManualJobInput{Title: strings.TrimSpace(request.Title), URL: strings.TrimSpace(request.URL)}

	if reference, isPostingURL := jobboards.ParsePostingURL(given.URL); isPostingURL && adding.Postings != nil {
		posting, err := adding.Postings.FetchPosting(ctx, reference)
		if err != nil {
			return addJobAsGiven(ctx, hub, given, describeUnreadPosting(reference, err))
		}
		return addProviderPosting(ctx, hub, reference, posting, given.URL)
	}
	if adding.Pages == nil {
		return addJobAsGiven(ctx, hub, given, "")
	}
	page, err := adding.Pages.ReadPage(ctx, given.URL)
	if err != nil {
		return addJobAsGiven(ctx, hub, given, "The hub couldn't read the page: "+err.Error()+".")
	}
	if page.Reference != nil && adding.Postings != nil {
		posting, err := adding.Postings.FetchPosting(ctx, *page.Reference)
		if err == nil {
			return addProviderPosting(ctx, hub, *page.Reference, posting, given.URL)
		}
		slog.Info("the provider posting a page links to couldn't be read", "url", given.URL, "provider", page.Reference.Provider, "error", err)
	}
	read := page.Posting
	if strings.TrimSpace(read.Description) == "" {
		return addJobAsGiven(ctx, hub, given, "The page shows no posting text without JavaScript, so the hub couldn't read it.")
	}
	title := given.Title
	if title == "" {
		title = read.Title
	}
	if title == "" {
		return store.Job{}, false, errors.New("the page names no title; give one to store the job")
	}
	return hub.AddManualJob(ctx, owner, store.ManualJobInput{
		Title: title, URL: given.URL, Location: read.Location, WorkplaceType: read.WorkplaceType, Description: read.Description,
		BoardFacts: read.BoardFacts,
	})
}

// owner is the actor of the owner's own adds.
var owner = store.Actor{Kind: store.ActorOwner}

// addProviderPosting stores a posting read from its provider: as its board's
// job when the board is stored, else as a job added by hand that names it.
func addProviderPosting(ctx context.Context, hub *store.Store, reference jobboards.PostingReference, posting store.JobPosting,
	givenURL string) (store.Job, bool, error) {
	board, err := hub.GetJobBoardByToken(ctx, reference.Provider, reference.BoardToken)
	if errors.Is(err, store.ErrJobBoardNotFound) {
		jobURL := posting.URL
		if jobURL == "" {
			jobURL = givenURL
		}
		externalID := posting.ExternalID
		if externalID == "" {
			externalID = reference.PostingID
		}
		return hub.AddManualJob(ctx, owner, store.ManualJobInput{
			Title: posting.Title, URL: jobURL, Location: posting.Location, WorkplaceType: posting.WorkplaceType, Description: posting.Description,
			BoardFacts: posting.BoardFacts, Raw: posting.Raw,
			Posting: &store.ProviderPosting{Provider: reference.Provider, BoardToken: reference.BoardToken, ExternalID: externalID},
		})
	}
	if err != nil {
		return store.Job{}, false, err
	}
	return hub.UpsertBoardJob(ctx, owner, board, posting, time.Now())
}

// addJobAsGiven stores the job with the title and URL given, and the reason
// it has no text. Without a title it isn't stored, and the error says why.
func addJobAsGiven(ctx context.Context, hub *store.Store, given store.ManualJobInput, reason string) (store.Job, bool, error) {
	if given.Title == "" {
		if reason == "" {
			return store.Job{}, false, errors.New("a job needs a title and an http(s) URL")
		}
		return store.Job{}, false, errors.New(reason + " Give a title to store the job as given.")
	}
	given.TextMissingReason = reason
	return hub.AddManualJob(ctx, owner, given)
}

// describeUnreadPosting says why a provider's posting wasn't read.
func describeUnreadPosting(reference jobboards.PostingReference, err error) string {
	provider := strings.ToUpper(reference.Provider[:1]) + reference.Provider[1:]
	return "The hub couldn't read the posting on " + provider + ": " + err.Error() + "."
}
