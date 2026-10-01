package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// RegisterCVRoutes adds the owner-only routes for the base CV and for a CV
// rendered as HTML in the owner's design.
func RegisterCVRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("GET /v1/cvs/base", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cv, err := hub.GetBaseCV(r.Context())
		writeCVOrError(w, cv, err)
	})))
	routes.Handle("PUT /v1/cvs/base", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var content resume.Resume
		if err := json.NewDecoder(r.Body).Decode(&content); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be a JSON Resume: " + err.Error()})
			return
		}
		if content.Basics.Name == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the CV needs basics.name"})
			return
		}
		cv, err := hub.SaveBaseCV(r.Context(), owner, content)
		writeCVOrError(w, cv, err)
	})))
	routes.Handle("GET /v1/cvs/{id}/html", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cv store.CV
		var err error
		if r.PathValue("id") == store.CVKindBase {
			cv, err = hub.GetBaseCV(r.Context())
		} else if id, parseErr := uuid.Parse(r.PathValue("id")); parseErr != nil {
			err = store.ErrCVNotFound
		} else {
			cv, err = hub.GetCV(r.Context(), id)
		}
		if err != nil {
			writeCVOrError(w, cv, err)
			return
		}
		page, err := resume.RenderHTML(cv.Content)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})))
}

func writeCVOrError(w http.ResponseWriter, cv store.CV, err error) {
	switch {
	case errors.Is(err, store.ErrCVNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "no such CV"})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusOK, cv)
	}
}
