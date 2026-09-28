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
	NewHealthHandler(fakePinger{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != `{"database":"ok"}` {
		t.Fatalf("body = %s", body)
	}
}

func TestHealthHandlerReportsAnUnreachableDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHealthHandler(fakePinger{err: errors.New("connection refused")}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/health", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != `{"database":"unreachable"}` {
		t.Fatalf("body = %s", body)
	}
}
