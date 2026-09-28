package jobboards_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
)

func startProviders(t *testing.T) *jobboards.Verifier {
	t.Helper()
	routes := http.NewServeMux()
	routes.HandleFunc("GET /v1/boards/acme/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"jobs":[{"id":1},{"id":2}],"meta":{"total":2}}`))
	})
	routes.HandleFunc("GET /v0/postings/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[{"id":"a"}]`))
	})
	routes.HandleFunc("GET /posting-api/job-board/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"jobs":[{},{},{}],"apiVersion":"1"}`))
	})
	routes.HandleFunc("GET /pageonly", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html><script>window.__appData = {"organization":{"hostedJobsPageSlug":"pageonly"}}</script></html>`))
	})
	routes.HandleFunc("GET /nobody", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html><script>window.__appData = {"organization":null}</script></html>`))
	})
	routes.HandleFunc("GET /v1/boards/slow/jobs", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	})
	routes.HandleFunc("GET /v1/boards/broken/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)

	return &jobboards.Verifier{
		HTTPClient:        &http.Client{Timeout: 100 * time.Millisecond},
		GreenhouseAPIBase: server.URL,
		LeverAPIBase:      server.URL,
		AshbyAPIBase:      server.URL,
		AshbyBoardBase:    server.URL,
	}
}

func countOf(verification jobboards.Verification) string {
	if verification.OpenPostingCount == nil {
		return "unknown"
	}
	return strconv.Itoa(*verification.OpenPostingCount)
}

func TestVerifyCountsOpenPostingsPerProvider(t *testing.T) {
	verifier := startProviders(t)
	for provider, want := range map[string]struct{ count, boardURL string }{
		jobboards.Greenhouse: {"2", "https://job-boards.greenhouse.io/acme"},
		jobboards.Lever:      {"1", "https://jobs.lever.co/acme"},
		jobboards.Ashby:      {"3", "https://jobs.ashbyhq.com/acme"},
	} {
		got, err := verifier.Verify(context.Background(), provider, "acme")
		if err != nil || !got.Verified || countOf(got) != want.count || got.BoardURL != want.boardURL {
			t.Errorf("%s: got %+v (count %s), %v; want %+v", provider, got, countOf(got), err, want)
		}
	}
}

func TestVerifyFallsBackToTheAshbyBoardPage(t *testing.T) {
	verifier := startProviders(t)

	got, err := verifier.Verify(context.Background(), jobboards.Ashby, "pageonly")
	if err != nil || !got.Verified || got.OpenPostingCount != nil || got.BoardURL != "https://jobs.ashbyhq.com/pageonly" {
		t.Fatalf("page-only board: got %+v, %v; want verified with no count", got, err)
	}
	if got, err := verifier.Verify(context.Background(), jobboards.Ashby, "nobody"); err != nil || got.Verified {
		t.Fatalf("a page without the slug: got %+v, %v; want unverified", got, err)
	}
}

func TestVerifyReportsAnUnknownBoardAsUnverified(t *testing.T) {
	got, err := startProviders(t).Verify(context.Background(), jobboards.Greenhouse, "nobody")
	if err != nil || got.Verified {
		t.Fatalf("got %+v, %v; want unverified with no error", got, err)
	}
}

func TestVerifyFailsOnTimeoutsAndServerErrors(t *testing.T) {
	verifier := startProviders(t)
	for _, boardToken := range []string{"slow", "broken"} {
		if _, err := verifier.Verify(context.Background(), jobboards.Greenhouse, boardToken); err == nil {
			t.Errorf("%s: expected an error", boardToken)
		}
	}
}

func TestVerifyRefusesUnsupportedProviders(t *testing.T) {
	_, err := startProviders(t).Verify(context.Background(), "workday", "acme")
	if !errors.Is(err, jobboards.ErrUnsupportedProvider) {
		t.Fatalf("err = %v, want ErrUnsupportedProvider", err)
	}
}

// TestLiveBoards calls the real providers, so it only runs when asked.
func TestLiveBoards(t *testing.T) {
	if os.Getenv("HUB_LIVE_TEST") != "1" {
		t.Skip("set HUB_LIVE_TEST=1 to call the real provider APIs")
	}
	verifier := jobboards.NewVerifier()
	for provider, boardToken := range map[string]string{
		jobboards.Greenhouse: "stripe",
		jobboards.Lever:      "palantir",
		jobboards.Ashby:      "ramp",
	} {
		got, err := verifier.Verify(context.Background(), provider, boardToken)
		if err != nil || !got.Verified || got.OpenPostingCount == nil || *got.OpenPostingCount == 0 {
			t.Errorf("%s/%s: got %+v, %v", provider, boardToken, got, err)
		}
	}
	// Cherry Technologies turns Ashby's posting API off; its board page still
	// confirms the board.
	if got, err := verifier.Verify(context.Background(), jobboards.Ashby, "withcherry"); err != nil || !got.Verified || got.OpenPostingCount != nil {
		t.Errorf("ashby/withcherry: got %+v, %v; want verified with no count", got, err)
	}
}
