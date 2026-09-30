package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// fullBriefTimeout bounds one full brief written in the background.
const fullBriefTimeout = 5 * time.Minute

type fullBriefWriter interface {
	WriteFullBrief(ctx context.Context, jobID uuid.UUID) error
}

type fullBriefResponse struct {
	Queued bool `json:"queued"`
}

// RegisterJobBriefRoutes adds the owner-only route that asks Claude for a
// job's full brief. The brief is written in the background; the job's
// details show it once it's saved. A nil writer answers that full briefs
// are off.
func RegisterJobBriefRoutes(routes *http.ServeMux, hub *store.Store, writer fullBriefWriter, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("POST /v1/jobs/{id}/brief/full", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathIDOrWriteNotFound(w, r)
		if !ok {
			return
		}
		if writer == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "full briefs are off: the hub found no Claude CLI"})
			return
		}
		if _, err := hub.GetJobToBrief(r.Context(), id); errors.Is(err, store.ErrJobNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), fullBriefTimeout)
			defer cancel()
			if err := writer.WriteFullBrief(ctx, id); err != nil {
				slog.Warn("full brief failed", "job", id, "error", err)
			}
		}()
		writeJSON(w, http.StatusAccepted, fullBriefResponse{Queued: true})
	})))
}
