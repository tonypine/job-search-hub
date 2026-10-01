package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

func TestTheMarketGapsAreServedMostAskedFirst(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	routes := http.NewServeMux()
	api.RegisterMarketGapRoutes(routes, hub, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	now := time.Now()
	if err := hub.SaveMarketGaps(context.Background(), []store.MarketGap{
		{Technology: "AWS", JobCount: 3, GoodFits: 10, JobIDs: []uuid.UUID{uuid.New()}, ComputedAt: now},
		{Technology: "GraphQL", JobCount: 7, GoodFits: 10, JobIDs: []uuid.UUID{uuid.New()}, PlanKind: "learn", Plan: "Take the course.", ComputedAt: now},
	}); err != nil {
		t.Fatal(err)
	}

	status, body := send(t, http.MethodGet, server.URL+"/v1/market-gaps", ownerToken, "")
	var served struct {
		Gaps []store.MarketGap `json:"gaps"`
	}
	if json.Unmarshal(body, &served); status != http.StatusOK || len(served.Gaps) != 2 || served.Gaps[0].Technology != "GraphQL" || served.Gaps[0].Plan != "Take the course." {
		t.Fatalf("gaps: %d %s", status, body)
	}
}
