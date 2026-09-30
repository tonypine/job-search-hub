package api

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const defaultSignalDays = 7

type decisionQueueItem struct {
	Job         store.Job          `json:"job"`
	CompanyName *string            `json:"company_name,omitempty"`
	Match       string             `json:"match"`
	Reason      string             `json:"reason"`
	BriefTier   string             `json:"brief_tier"`
	Fit         jobfit.Fit         `json:"fit"`
	Decision    *store.JobDecision `json:"decision,omitempty"`
}

type decisionQueueResponse struct {
	Items []decisionQueueItem `json:"items"`
	Total int                 `json:"total"`
}

// decisionSignals say how deciding goes: the decisions made in the period by
// kind, how long jobs waited for them, and how many of the good and unclear
// jobs seen in the period are decided.
type decisionSignals struct {
	Since     time.Time      `json:"since"`
	Decisions map[string]int `json:"decisions"`
	// MedianHoursToDecide is the median time from a job first seen to its
	// decision, for the decisions made in the period; nil without any.
	MedianHoursToDecide  *float64 `json:"median_hours_to_decide,omitempty"`
	GoodOrUnclearSeen    int      `json:"good_or_unclear_seen"`
	GoodOrUnclearDecided int      `json:"good_or_unclear_decided"`
}

// RegisterDecisionRoutes adds the owner-only routes for the decision queue
// and the decision signals.
func RegisterDecisionRoutes(routes *http.ServeMux, hub *store.Store, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/decision-queue", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		queued, err := hub.ListDecisionQueue(r.Context(), startOfToday)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		criteria, rates, err := jobfit.ReadInputs(r.Context(), hub, rateSource)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		items := make([]decisionQueueItem, 0, len(queued))
		for _, queuedItem := range queued {
			items = append(items, decisionQueueItem{
				Job: queuedItem.Job, CompanyName: queuedItem.CompanyName, Match: queuedItem.Match, Reason: queuedItem.Reason, BriefTier: queuedItem.BriefTier,
				Fit: jobfit.Judge(queuedItem.Job, queuedItem.Facts, criteria, rates), Decision: queuedItem.Decision,
			})
		}
		writeJSON(w, http.StatusOK, decisionQueueResponse{Items: items, Total: len(items)})
	})))

	routes.Handle("GET /v1/decision-signals", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		days := defaultSignalDays
		if raw := r.URL.Query().Get("days"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "days must be a positive number"})
				return
			}
			days = parsed
		}
		since := time.Now().AddDate(0, 0, -days)
		signals, err := getDecisionSignals(r, hub, rateSource, since)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, signals)
	})))
}

func getDecisionSignals(r *http.Request, hub *store.Store, rateSource exchangeRateSource, since time.Time) (decisionSignals, error) {
	signals := decisionSignals{Since: since, Decisions: map[string]int{store.JobDecisionPursue: 0, store.JobDecisionSkip: 0, store.JobDecisionLater: 0}}
	decisions, err := hub.ListDecisionsSince(r.Context(), since)
	if err != nil {
		return decisionSignals{}, err
	}
	var waits []float64
	for _, decision := range decisions {
		signals.Decisions[decision.Decision]++
		waits = append(waits, decision.DecidedAt.Sub(decision.FirstSeenAt).Hours())
	}
	if len(waits) > 0 {
		median := getMedian(waits)
		signals.MedianHoursToDecide = &median
	}

	seen, err := hub.ListJobsSeenSince(r.Context(), since)
	if err != nil {
		return decisionSignals{}, err
	}
	criteria, rates, err := jobfit.ReadInputs(r.Context(), hub, rateSource)
	if err != nil {
		return decisionSignals{}, err
	}
	for _, job := range seen {
		if jobfit.Judge(job.Job, job.Facts, criteria, rates).Level == jobfit.LevelPoor {
			continue
		}
		signals.GoodOrUnclearSeen++
		if job.IsDecided {
			signals.GoodOrUnclearDecided++
		}
	}
	return signals, nil
}

func getMedian(values []float64) float64 {
	sorted := slices.Sorted(slices.Values(values))
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}
