package hubclients

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseReadsThePlatformAndVersion(t *testing.T) {
	for header, want := range map[string]Client{
		"macos/0.1.252":               {Platform: MacOS, Version: "0.1.252"},
		" android/0.1.0-dev.4c1e8a9 ": {Platform: Android, Version: "0.1.0-dev.4c1e8a9"},
		"":                            {},
		"macos":                       {},
		"macos/":                      {},
		"/0.1.2":                      {},
		"android/0.1.2 (Pixel 8)":     {},
		"android/0.1.2/extra":         {},
	} {
		got, found := Parse(header)
		if got != want || found != (want != Client{}) {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", header, got, found, want)
		}
	}
}

func serve(gate Gate, path, client string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if client != "" {
		request.Header.Set(Header, client)
	}
	recorder := httptest.NewRecorder()
	gate.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(recorder, request)
	return recorder
}

func TestTheGateTurnsAwayAnAppOlderThanItsPlatformsMinimum(t *testing.T) {
	gate := Gate{Minimums: map[string]string{Android: "0.1.200"}, ServerVersion: "0.1.250"}

	refused := serve(gate, "/v1/jobs", "android/0.1.199")
	if refused.Code != http.StatusUpgradeRequired || refused.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("an old phone: %d %s", refused.Code, refused.Body.String())
	}
	if body := strings.TrimSpace(refused.Body.String()); body != `{"error":"This app (0.1.199) is too old for the hub, which runs 0.1.250. Update the app to 0.1.200 or later."}` {
		t.Fatalf("body = %s", body)
	}

	for _, served := range []struct{ path, client string }{
		{"/v1/jobs", "android/0.1.200"},
		{"/v1/jobs", "android/0.2.0"},
		{"/v1/jobs", "android/0.1.0-dev.4c1e8a9"},
		{"/v1/jobs", "macos/0.1.1"},
		{"/v1/jobs", ""},
		{"/v1/health", "android/0.1.1"},
		{"/v1/version", "android/0.1.1"},
	} {
		if code := serve(gate, served.path, served.client).Code; code != http.StatusOK {
			t.Errorf("%s from %q: %d, want 200", served.path, served.client, code)
		}
	}
}

func TestWithNoMinimumsEveryAppIsServed(t *testing.T) {
	if code := serve(Gate{}, "/v1/jobs", "android/0.0.1").Code; code != http.StatusOK {
		t.Fatalf("%d, want 200", code)
	}
}
