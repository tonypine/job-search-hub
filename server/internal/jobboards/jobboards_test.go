package jobboards_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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
	}
}

func TestVerifyCountsOpenPostingsPerProvider(t *testing.T) {
	verifier := startProviders(t)
	for provider, want := range map[string]jobboards.Verification{
		jobboards.Greenhouse: {Verified: true, OpenPostingCount: 2, BoardURL: "https://job-boards.greenhouse.io/acme"},
		jobboards.Lever:      {Verified: true, OpenPostingCount: 1, BoardURL: "https://jobs.lever.co/acme"},
		jobboards.Ashby:      {Verified: true, OpenPostingCount: 3, BoardURL: "https://jobs.ashbyhq.com/acme"},
	} {
		got, err := verifier.Verify(context.Background(), provider, "acme")
		if err != nil || got != want {
			t.Errorf("%s: got %+v, %v; want %+v", provider, got, err, want)
		}
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
		if err != nil || !got.Verified || got.OpenPostingCount == 0 {
			t.Errorf("%s/%s: got %+v, %v", provider, boardToken, got, err)
		}
	}
}
