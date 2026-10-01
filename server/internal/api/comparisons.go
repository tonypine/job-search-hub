package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/comparisons"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// comparisonRunTimeout bounds one comparison run in the background.
	comparisonRunTimeout = 2 * time.Hour
	// maximumFreshJobs bounds the fresh jobs one comparison asks for.
	maximumFreshJobs = 50
)

type comparisonRunner interface {
	Run(ctx context.Context, comparisonID uuid.UUID) error
}

type comparisonsResponse struct {
	Comparisons []store.Comparison `json:"comparisons"`
}

// comparisonResponse carries each answer's readings beside it; its Answers
// stand in for the record's.
type comparisonResponse struct {
	store.ComparisonRecord
	Answers []answerWithReadings `json:"answers"`
	Summary comparisons.Summary  `json:"summary"`
}

type answerWithReadings struct {
	store.ComparisonAnswer
	Readings []comparisons.Reading `json:"readings"`
}

type importedStack struct {
	Label   string                   `json:"label"`
	Model   string                   `json:"model"`
	Answers []store.ComparisonAnswer `json:"answers"`
}

type importComparisonRequest struct {
	TaskKind string          `json:"task_kind"`
	Title    string          `json:"title"`
	JobIDs   []uuid.UUID     `json:"job_ids"`
	Stacks   []importedStack `json:"stacks"`
}

type stackToRun struct {
	Label      string     `json:"label"`
	Source     string     `json:"source"`
	ProviderID *uuid.UUID `json:"provider_id"`
	Model      string     `json:"model"`
}

type createComparisonRequest struct {
	Title     string       `json:"title"`
	JobIDs    []uuid.UUID  `json:"job_ids"`
	FreshJobs int          `json:"fresh_jobs"`
	Stacks    []stackToRun `json:"stacks"`
}

type comparisonVerdictsRequest struct {
	Verdicts []store.ComparisonVerdict `json:"verdicts"`
}

// RegisterComparisonRoutes adds the owner-only routes that list, import, run
// and judge model comparisons. A new comparison runs in the background; its
// answers show up as they land. A nil runner still lists, imports and judges.
func RegisterComparisonRoutes(routes *http.ServeMux, hub *store.Store, runner comparisonRunner, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("GET /v1/comparisons", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list, err := hub.ListComparisons(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, comparisonsResponse{Comparisons: list})
	})))
	routes.Handle("GET /v1/comparisons/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		writeComparison(w, r.Context(), hub, id, http.StatusOK)
	})))
	routes.Handle("POST /v1/comparisons/import", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request importComparisonRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		input := store.NewComparison{TaskKind: request.TaskKind, Title: strings.TrimSpace(request.Title), JobIDs: request.JobIDs}
		for _, stack := range request.Stacks {
			input.Stacks = append(input.Stacks, store.ComparisonStack{Label: stack.Label, Source: store.ComparisonSourceImported, Model: stack.Model})
		}
		comparison, ok := createComparisonOrWriteError(w, r.Context(), hub, owner, input)
		if !ok {
			return
		}
		for position, stack := range request.Stacks {
			for _, answer := range stack.Answers {
				answer.StackID = comparison.Stacks[position].ID
				if err := hub.SaveComparisonAnswer(r.Context(), answer); err != nil {
					writeJSON(w, http.StatusBadRequest, errorResponse{Error: "save an answer of " + stack.Label + ": " + err.Error()})
					return
				}
			}
		}
		if err := hub.FinishComparison(r.Context(), comparison.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeComparison(w, r.Context(), hub, comparison.ID, http.StatusCreated)
	})))
	routes.Handle("POST /v1/comparisons", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if runner == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "comparisons can't run: the hub has no model routes"})
			return
		}
		var request createComparisonRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		if request.FreshJobs < 0 || request.FreshJobs > maximumFreshJobs {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "fresh_jobs is from 0 to 50"})
			return
		}
		input := store.NewComparison{TaskKind: store.AgentPromptKindJobFacts, Title: strings.TrimSpace(request.Title), JobIDs: request.JobIDs}
		if request.FreshJobs > 0 {
			fresh, err := hub.ListNewestJobIDsWithText(r.Context(), request.FreshJobs)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			input.JobIDs = append(input.JobIDs, fresh...)
		}
		for _, stack := range request.Stacks {
			if stack.Source != store.ComparisonSourceRoute && stack.Source != store.ComparisonSourceClaude {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "a stack to run is a route or claude"})
				return
			}
			if stack.Source == store.ComparisonSourceRoute && (stack.ProviderID == nil || stack.Model == "") {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "a route stack needs provider_id and model"})
				return
			}
			input.Stacks = append(input.Stacks, store.ComparisonStack{Label: stack.Label, Source: stack.Source, ProviderID: stack.ProviderID, Model: stack.Model})
		}
		comparison, ok := createComparisonOrWriteError(w, r.Context(), hub, owner, input)
		if !ok {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), comparisonRunTimeout)
			defer cancel()
			if err := runner.Run(ctx, comparison.ID); err != nil {
				slog.Warn("comparison stopped", "comparison", comparison.ID, "error", err)
			}
		}()
		writeComparison(w, r.Context(), hub, comparison.ID, http.StatusAccepted)
	})))
	routes.Handle("PUT /v1/comparisons/{id}/verdicts", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		var request comparisonVerdictsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		for _, verdict := range request.Verdicts {
			err := hub.SaveComparisonVerdict(r.Context(), id, verdict)
			if errors.Is(err, store.ErrComparisonNotFound) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "no such stack in this comparison"})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
				return
			}
		}
		writeComparison(w, r.Context(), hub, id, http.StatusOK)
	})))
}

func createComparisonOrWriteError(w http.ResponseWriter, ctx context.Context, hub *store.Store, owner store.Actor, input store.NewComparison) (store.Comparison, bool) {
	if input.Title == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "a comparison needs a title"})
		return store.Comparison{}, false
	}
	for _, stack := range input.Stacks {
		if strings.TrimSpace(stack.Label) == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "every stack needs a label"})
			return store.Comparison{}, false
		}
	}
	comparison, err := hub.CreateComparison(ctx, owner, input)
	if errors.Is(err, store.ErrJobNotFound) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return store.Comparison{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return store.Comparison{}, false
	}
	return comparison, true
}

func writeComparison(w http.ResponseWriter, ctx context.Context, hub *store.Store, id uuid.UUID, status int) {
	record, err := hub.GetComparison(ctx, id)
	if errors.Is(err, store.ErrComparisonNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	response := comparisonResponse{ComparisonRecord: record, Summary: comparisons.Summarize(record), Answers: make([]answerWithReadings, len(record.Answers))}
	for index, answer := range record.Answers {
		response.Answers[index] = answerWithReadings{ComparisonAnswer: answer, Readings: comparisons.ListReadings(answer.Answer)}
	}
	writeJSON(w, status, response)
}
