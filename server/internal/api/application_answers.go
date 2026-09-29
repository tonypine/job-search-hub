package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type applicationAnswersResponse struct {
	Answers []store.ApplicationAnswer `json:"answers"`
}

type saveApplicationAnswerRequest struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// RegisterApplicationAnswerRoutes adds the owner-only routes for the library
// of answers to application form questions.
func RegisterApplicationAnswerRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("GET /v1/application-answers", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answers, err := hub.ListApplicationAnswers(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, applicationAnswersResponse{Answers: answers})
	})))

	save := func(w http.ResponseWriter, r *http.Request, id *uuid.UUID, successStatus int) {
		var request saveApplicationAnswerRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		saved, err := hub.SaveApplicationAnswer(r.Context(), owner, id, request.Question, request.Answer)
		switch {
		case errors.Is(err, store.ErrApplicationAnswerNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case errors.Is(err, store.ErrDuplicateQuestion):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case err != nil:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, successStatus, saved)
		}
	}
	routes.Handle("POST /v1/application-answers", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		save(w, r, nil, http.StatusCreated)
	})))
	routes.Handle("PUT /v1/application-answers/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		save(w, r, &id, http.StatusOK)
	})))
	routes.Handle("DELETE /v1/application-answers/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		err = hub.DeleteApplicationAnswer(r.Context(), owner, id)
		switch {
		case errors.Is(err, store.ErrApplicationAnswerNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})))
}
