package api

import (
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type marketGapsResponse struct {
	Gaps []store.MarketGap `json:"gaps"`
}

// RegisterMarketGapRoutes adds the owner-only route that lists the skills
// good fits keep asking for and the knowledge base lacks, with their plans.
func RegisterMarketGapRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/market-gaps", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gaps, err := hub.ListMarketGaps(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, marketGapsResponse{Gaps: gaps})
	})))
}
