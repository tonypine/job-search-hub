package openrouter_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/openrouter"
)

func TestDownBusyOrOutOfCreditsIsUnreachable(t *testing.T) {
	for status, unreachable := range map[int]bool{
		http.StatusBadRequest: false, http.StatusUnauthorized: false, http.StatusPaymentRequired: true, http.StatusForbidden: false,
		http.StatusRequestTimeout: true, http.StatusTooManyRequests: true, http.StatusBadGateway: true, http.StatusServiceUnavailable: true,
	} {
		err := openrouter.ReadError(status, []byte(`{"error":{"code":0,"message":"the reason","metadata":{"provider_name":"Anthropic"}}}`))
		if errors.Is(err, chatcompletions.ErrUnreachable) != unreachable || !strings.Contains(err.Error(), "the reason (Anthropic)") {
			t.Errorf("%d: %v; want unreachable %v", status, err, unreachable)
		}
	}
	if err := openrouter.ReadError(http.StatusBadGateway, []byte("Bad gateway")); !strings.Contains(err.Error(), "Bad gateway") {
		t.Errorf("an error that isn't OpenRouter's JSON: %v", err)
	}
}

func TestTheCatalogueComesSortedWithPricesPerMillionTokens(t *testing.T) {
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.Write([]byte(`{"data":[
			{"id":"z/model","name":"Z","pricing":{"prompt":"0.0000025","completion":"0.00001"},"context_length":128000},
			{"id":"openrouter/auto","name":"Auto Router","pricing":{"prompt":"-1","completion":"-1"}},
			{"id":"a/free","name":"Free","pricing":{"prompt":"0","completion":"0"}},
			{"id":"a/odd","name":"Odd","pricing":{"prompt":"","completion":"0.000000001"}}]}`))
	}))
	defer server.Close()

	models, err := openrouter.ListModels(context.Background(), server.URL+"/", "sk-or")
	if err != nil || len(models) != 4 {
		t.Fatalf("models = %+v, %v", models, err)
	}
	if headers.Get("Authorization") != "Bearer sk-or" || headers.Get("X-Title") != "Job Search Hub" || headers.Get("HTTP-Referer") == "" {
		t.Errorf("headers = %v", headers)
	}
	price := func(value *string) string {
		if value == nil {
			return "none"
		}
		return *value
	}
	var got []string
	for _, model := range models {
		got = append(got, model.ID+" "+price(model.InputUSDPerMillion)+"/"+price(model.OutputUSDPerMillion))
	}
	want := "a/free 0/0, a/odd none/0.001, openrouter/auto none/none, z/model 2.5/10"
	if strings.Join(got, ", ") != want {
		t.Errorf("models = %s\nwant      %s", strings.Join(got, ", "), want)
	}
	if models[3].Name != "Z" || models[3].ContextLength != 128000 {
		t.Errorf("z/model = %+v", models[3])
	}
}

func TestARefusedCatalogueSaysWhy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"code":401,"message":"User not found."}}`))
	}))
	defer server.Close()
	if _, err := openrouter.ListModels(context.Background(), server.URL, "sk-wrong"); err == nil || !strings.Contains(err.Error(), "OpenRouter refused the key: User not found.") {
		t.Fatalf("error = %v", err)
	}
}

func TestAConfiguredClientAsksAsOpenRouterAsks(t *testing.T) {
	client := chatcompletions.NewClient(openrouter.BaseURL)
	openrouter.Configure(client)
	if client.Headers["X-Title"] != "Job Search Hub" || !client.AskForCost || client.ReadError == nil {
		t.Fatalf("client = %+v", client)
	}
}
