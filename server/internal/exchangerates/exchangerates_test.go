package exchangerates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRatesAreFetchedOncePerHalfDayPerBase(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/latest" || r.URL.Query().Get("base") != "BRL" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"amount":1.0,"base":"BRL","date":"2026-09-28","rates":{"USD":0.19224,"EUR":0.16896}}`))
	}))
	defer server.Close()
	clock := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	cache := NewCache(server.URL)
	cache.now = func() time.Time { return clock }

	rates, err := cache.GetRates(context.Background(), "BRL")
	if err != nil || rates["USD"] != 0.19224 {
		t.Fatalf("rates = %v, %v", rates, err)
	}
	clock = clock.Add(6 * time.Hour)
	if _, err := cache.GetRates(context.Background(), "BRL"); err != nil || requests != 1 {
		t.Fatalf("after 6 hours: %d requests, %v; want the cached rates", requests, err)
	}
	clock = clock.Add(7 * time.Hour)
	if _, err := cache.GetRates(context.Background(), "BRL"); err != nil || requests != 2 {
		t.Fatalf("after 13 hours: %d requests, %v; want a fresh fetch", requests, err)
	}
	if _, err := cache.GetRates(context.Background(), "XYZ"); err == nil {
		t.Fatal("an unknown base returned rates")
	}
}
