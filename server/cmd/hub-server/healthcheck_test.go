package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunHealthcheckPassesOnOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			t.Errorf("probed %s, want /v1/health", r.URL.Path)
		}
	}))
	defer server.Close()

	if err := runHealthcheck(strings.TrimPrefix(server.URL, "http://")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunHealthcheckFailsOnUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := runHealthcheck(strings.TrimPrefix(server.URL, "http://")); err == nil {
		t.Fatal("expected an error for a 503")
	}
}

func TestRunHealthcheckProbesLoopbackForAnUnspecifiedHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	port := server.URL[strings.LastIndex(server.URL, ":")+1:]
	if err := runHealthcheck(":" + port); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
