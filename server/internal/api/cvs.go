package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/cvdrafts"
	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type cvDrafter interface {
	DraftCV(ctx context.Context, jobID uuid.UUID) (store.CV, error)
	SaveEdit(ctx context.Context, actor store.Actor, jobID uuid.UUID, edit cvdrafts.Draft) (store.CV, error)
}

// cvDraftTimeout bounds one tailored CV drafted in the background.
const cvDraftTimeout = 5 * time.Minute

// RegisterCVRoutes adds the owner-only routes for the base CV, a job's
// tailored CV, and any CV rendered as HTML in the owner's design. A nil
// drafter answers that drafting is off.
func RegisterCVRoutes(routes *http.ServeMux, hub *store.Store, drafter cvDrafter, requireOwner func(http.Handler) http.Handler) {
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
	routes.Handle("GET /v1/jobs/{id}/cv/screen", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		screen, err := hub.GetCVScreen(r.Context(), id)
		if errors.Is(err, store.ErrCVScreenNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "the job's CV hasn't been screened yet"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, screen)
	})))
	routes.Handle("GET /v1/jobs/{id}/cv", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeCVOrError(w, store.CV{}, store.ErrCVNotFound)
			return
		}
		cv, err := hub.GetJobCV(r.Context(), id)
		writeCVOrError(w, cv, err)
	})))
	routes.Handle("POST /v1/jobs/{id}/cv", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		if drafter == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "CV drafts are off: the hub found no Claude CLI"})
			return
		}
		if _, err := hub.GetJobToBrief(r.Context(), id); errors.Is(err, store.ErrJobNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), cvDraftTimeout)
			defer cancel()
			if _, err := drafter.DraftCV(ctx, id); err != nil {
				slog.Warn("CV draft failed", "job", id, "error", err)
			}
		}()
		writeJSON(w, http.StatusAccepted, fullBriefResponse{Queued: true})
	})))
	routes.Handle("PUT /v1/jobs/{id}/cv", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		if drafter == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "CV drafts are off: the hub found no Claude CLI"})
			return
		}
		var edit cvdrafts.Draft
		if err := json.NewDecoder(r.Body).Decode(&edit); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		cv, err := drafter.SaveEdit(r.Context(), owner, id, edit)
		if errors.Is(err, chatcompletions.ErrInvalidAnswer) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeCVOrError(w, cv, err)
	})))
	routes.Handle("PUT /v1/cvs/{id}/pdf", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		pdf, err := io.ReadAll(io.LimitReader(r.Body, store.MaximumCVPDFSize+1))
		if err != nil || len(pdf) > store.MaximumCVPDFSize || !bytes.HasPrefix(pdf, []byte("%PDF")) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "send a PDF of at most 5 MB"})
			return
		}
		if err := hub.SaveCVPDF(r.Context(), owner, id, pdf); err != nil {
			writeCVOrError(w, store.CV{}, err)
			return
		}
		cv, err := hub.GetCV(r.Context(), id)
		writeCVOrError(w, cv, err)
	})))
	routes.Handle("GET /v1/cvs/{id}/pdf", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		pdf, err := hub.GetCVPDF(r.Context(), id)
		if err != nil {
			writeCVOrError(w, store.CV{}, err)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
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
