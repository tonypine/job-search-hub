package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/comparisons"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	FirstStepProfileInterview = "profile_interview"
	FirstStepTailoredCV       = "tailored_cv"
	FirstStepModelComparison  = "model_comparison"

	// confirmedEntriesToTailor is how many confirmed profile entries a
	// tailored CV wants to cite before it's worth drafting one.
	confirmedEntriesToTailor = 5
)

// FirstStep is a feature that gives nothing until the owner first uses it.
// Each is read from the hub's data, so it leaves once the owner has done it.
type FirstStep struct {
	Kind string `json:"kind"`
	// ConfirmedEntries is how many profile entries the owner has confirmed,
	// on the interview's step.
	ConfirmedEntries int `json:"confirmed_entries,omitempty"`
	// ComparisonID and ComparisonTitle name the comparison to judge, and
	// UnjudgedFields how many of its fields have no verdict yet.
	ComparisonID    *uuid.UUID `json:"comparison_id,omitempty"`
	ComparisonTitle string     `json:"comparison_title,omitempty"`
	UnjudgedFields  int        `json:"unjudged_fields,omitempty"`
}

type firstStepsResponse struct {
	Steps []FirstStep `json:"steps"`
}

// RegisterFirstStepRoutes adds the owner-only route that lists the features
// waiting on the owner's first use, for Today to prompt.
func RegisterFirstStepRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/first-steps", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		steps, err := listFirstSteps(r.Context(), hub)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, firstStepsResponse{Steps: steps})
	})))
}

// listFirstSteps reads each step's condition: the interview while fewer than
// five entries are confirmed, a tailored CV once they are and none is drafted,
// and the newest finished comparison with a field no verdict covers.
func listFirstSteps(ctx context.Context, hub *store.Store) ([]FirstStep, error) {
	steps := []FirstStep{}
	confirmed, err := hub.CountConfirmedProfileEntries(ctx)
	if err != nil {
		return nil, err
	}
	if confirmed < confirmedEntriesToTailor {
		steps = append(steps, FirstStep{Kind: FirstStepProfileInterview, ConfirmedEntries: confirmed})
	} else {
		hasCV, err := hub.HasTailoredCV(ctx)
		if err != nil {
			return nil, err
		}
		if !hasCV {
			steps = append(steps, FirstStep{Kind: FirstStepTailoredCV})
		}
	}
	comparison, err := findUnjudgedComparison(ctx, hub)
	if err != nil || comparison == nil {
		return steps, err
	}
	return append(steps, *comparison), nil
}

// findUnjudgedComparison returns the step for the newest finished comparison
// that has a field with no verdict on any stack or posting, or nil when every
// field of every one has one. A running comparison is still getting answers.
func findUnjudgedComparison(ctx context.Context, hub *store.Store) (*FirstStep, error) {
	list, err := hub.ListComparisons(ctx)
	if err != nil {
		return nil, err
	}
	for _, comparison := range list {
		if comparison.Status != store.ComparisonStatusDone {
			continue
		}
		record, err := hub.GetComparison(ctx, comparison.ID)
		if err != nil {
			return nil, err
		}
		judged := map[string]bool{}
		summary := comparisons.Summarize(record)
		for _, stack := range summary.Stacks {
			for _, score := range stack.Fields {
				if score.Right+score.Wrong > 0 {
					judged[score.Field] = true
				}
			}
		}
		if unjudged := len(summary.Fields) - len(judged); unjudged > 0 {
			return &FirstStep{Kind: FirstStepModelComparison, ComparisonID: &comparison.ID, ComparisonTitle: comparison.Title, UnjudgedFields: unjudged}, nil
		}
	}
	return nil, nil
}
