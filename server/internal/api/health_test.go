package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePinger struct{ err error }

func (pinger fakePinger) Ping(context.Context) error { return pinger.err }

func TestHealthHandlerReportsAReachableDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHealthHandler(fakePinger{}, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != `{"database":"ok"}` {
		t.Fatalf("body = %s", body)
	}
}

func TestHealthHandlerReportsAnUnreachableDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHealthHandler(fakePinger{err: errors.New("connection refused")}, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/health", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != `{"database":"unreachable"}` {
		t.Fatalf("body = %s", body)
	}
}

func TestHealthHandlerSaysWhichMajorItsOwnPostgresIsOn(t *testing.T) {
	for _, test := range []struct {
		postgres *PostgresHealth
		want     string
	}{
		{&PostgresHealth{Major: 18}, `{"database":"ok","postgres":{"major":18}}`},
		{&PostgresHealth{Major: 17, UpgradeFailedTo: 18}, `{"database":"ok","postgres":{"major":17,"upgrade_failed_to":18}}`},
	} {
		recorder := httptest.NewRecorder()
		NewHealthHandler(fakePinger{}, test.postgres).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
		if body := strings.TrimSpace(recorder.Body.String()); recorder.Code != http.StatusOK || body != test.want {
			t.Errorf("%d %s, want 200 %s", recorder.Code, body, test.want)
		}
	}
}
