package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type updateRecorder interface {
	Record(ctx context.Context, input store.NewUpdate) (store.Update, error)
}

type queueTaskRequest struct {
	Kind      string     `json:"kind"`
	CompanyID *uuid.UUID `json:"company_id"`
	Company   string     `json:"company"`
}

type finishTaskRequest struct {
	Succeeded bool       `json:"succeeded"`
	Result    string     `json:"result"`
	CompanyID *uuid.UUID `json:"company_id"`
}

type tasksResponse struct {
	Tasks []store.TaskRequest `json:"tasks"`
}

// RegisterTaskRoutes adds the routes for work the phone asks of the Mac: the
// phone queues it, the Mac app claims and runs it, and each step is an update
// both see.
func RegisterTaskRoutes(routes *http.ServeMux, hub *store.Store, updates updateRecorder, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("POST /v1/tasks", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request queueTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		var deviceID *uuid.UUID
		if id, found := tokens.GetDeviceID(r.Context()); found {
			deviceID = &id
		}
		task, err := hub.QueueTask(r.Context(), owner, request.Kind, request.CompanyID, request.Company, deviceID)
		switch {
		case errors.Is(err, store.ErrCompanyNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "no such company"})
			return
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		title := "From your phone: research " + task.Input
		if task.Kind == store.TaskFindJobs {
			title = "From your phone: find jobs at " + getCompanyName(r.Context(), hub, task.CompanyID)
		}
		if _, err := updates.Record(r.Context(), store.NewUpdate{
			Kind: "task_queued", Title: title, Body: "Waiting for the Mac to pick it up.", CompanyID: task.CompanyID,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, task)
	})))

	routes.Handle("GET /v1/tasks", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		tasks, err := hub.ListTasks(r.Context(), r.URL.Query().Get("status"), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, tasksResponse{Tasks: tasks})
	})))

	routes.Handle("POST /v1/tasks/{id}/claim", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		task, err := hub.ClaimTask(r.Context(), id)
		switch {
		case errors.Is(err, store.ErrTaskNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case errors.Is(err, store.ErrTaskNotQueued):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, task)
		}
	})))

	routes.Handle("POST /v1/tasks/{id}/finish", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		var request finishTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		task, err := hub.FinishTask(r.Context(), id, request.Succeeded, request.Result, request.CompanyID)
		switch {
		case errors.Is(err, store.ErrTaskNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		case err != nil:
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
			return
		}
		title, body, _ := strings.Cut(task.Result, "\n")
		if title == "" {
			title = "The Mac finished your request"
		}
		if !request.Succeeded {
			title, body = "The Mac couldn't finish your request", task.Result
		}
		if _, err := updates.Record(r.Context(), store.NewUpdate{Kind: "task_finished", Title: title, Body: strings.TrimSpace(body), CompanyID: task.CompanyID}); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, task)
	})))
}

func getCompanyName(ctx context.Context, hub *store.Store, companyID *uuid.UUID) string {
	if companyID == nil {
		return "the company"
	}
	company, err := hub.GetCompany(ctx, *companyID)
	if err != nil {
		return "the company"
	}
	return company.Name
}
