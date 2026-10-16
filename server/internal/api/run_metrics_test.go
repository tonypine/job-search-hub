package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func getRunMetrics(t *testing.T, service apiUnderTest, query url.Values) store.RunMetrics {
	t.Helper()
	status, answer := send(t, http.MethodGet, service.url+"/v1/runs/metrics?"+query.Encode(), ownerToken, "")
	var metrics store.RunMetrics
	if err := json.Unmarshal(answer, &metrics); err != nil || status != http.StatusOK {
		t.Fatalf("metrics %s: %d %s", query.Encode(), status, answer)
	}
	return metrics
}

func TestJobBriefsMetricsOverADayByTheHour(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	// India is 5:30 ahead of UTC and has no daylight saving, so a day is
	// always 24 of its hours, each starting on the half hour in UTC.
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(kolkata)
	to := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, kolkata)
	from := to.Add(-24 * time.Hour)
	for _, run := range []struct {
		kind    string
		at      time.Time
		outcome string
	}{
		{store.AgentPromptKindJobBrief, from, store.RunOutcomeSucceeded},
		{store.AgentPromptKindJobBrief, from.Add(30 * time.Minute), store.RunOutcomeFailed},
		{store.AgentPromptKindJobBrief, from.Add(5 * time.Hour), store.RunOutcomeInvalid},
		{store.AgentPromptKindJobBrief, to.Add(-time.Minute), store.RunOutcomeSucceeded},
		{store.AgentPromptKindJobBrief, from.Add(-time.Minute), store.RunOutcomeSucceeded},
		{store.AgentPromptKindJobBrief, to, store.RunOutcomeSucceeded},
		{store.AgentPromptKindJobFacts, from.Add(time.Hour), store.RunOutcomeSucceeded},
	} {
		if _, err := service.hub.RecordTaskRun(ctx, store.NewTaskRun{
			Kind: run.kind, Provider: "Local model", Model: "qwen", StartedAt: run.at, Duration: time.Second, Outcome: run.outcome,
			PromptTokens: 700, CompletionTokens: 90,
		}); err != nil {
			t.Fatal(err)
		}
	}

	window := url.Values{"from": {from.Format(time.RFC3339)}, "to": {to.Format(time.RFC3339)}}
	query := url.Values{"kind": {"job_brief"}, "bucket": {"hour"}, "time_zone": {"Asia/Kolkata"}}
	for name, values := range window {
		query[name] = values
	}
	metrics := getRunMetrics(t, service, query)
	if len(metrics.Buckets) != 24 {
		t.Fatalf("%d buckets; want 24", len(metrics.Buckets))
	}
	for index, bucket := range metrics.Buckets {
		start := from.Add(time.Duration(index) * time.Hour)
		if !bucket.Start.Equal(start) || !bucket.End.Equal(start.Add(time.Hour)) {
			t.Errorf("bucket %d runs from %v to %v; want the hour from %v", index, bucket.Start, bucket.End, start)
		}
	}
	if first := metrics.Buckets[0]; first.Runs != 2 || first.Failed != 1 || len(first.Kinds) != 1 || first.Kinds[0].Kind != "job_brief" {
		t.Errorf("the first hour = %+v", first)
	}

	page := listRuns(t, service, url.Values{"kind": {"job_brief"}, "since": window["from"], "until": window["to"], "limit": {"200"}})
	failed, invalid, inputTokens, outputTokens := 0, 0, int64(0), int64(0)
	for _, run := range page.Runs {
		switch run.Outcome {
		case store.RunOutcomeFailed:
			failed++
		case store.RunOutcomeInvalid:
			invalid++
		}
		inputTokens += int64(run.PromptTokens)
		outputTokens += int64(run.CompletionTokens)
	}
	if totals := metrics.Totals; len(page.Runs) != 4 || totals.Runs != len(page.Runs) || totals.Failed != failed || totals.Invalid != invalid ||
		totals.InputTokens != inputTokens || totals.OutputTokens != outputTokens {
		t.Errorf("totals = %+v; the window lists %d runs, %d failed, %d invalid, %d and %d tokens",
			totals, len(page.Runs), failed, invalid, inputTokens, outputTokens)
	}
	if previous := metrics.Previous; previous.Runs != 1 || !previous.To.Equal(from) || !previous.From.Equal(from.Add(-24*time.Hour)) {
		t.Errorf("previous = %+v", previous)
	}
	if metrics.Totals.CostUSD != "0" || metrics.TimeZone != "Asia/Kolkata" || metrics.Kind != "job_brief" {
		t.Errorf("metrics = %+v", metrics)
	}
}

func TestRunMetricsRefuseAQueryTheyCantRead(t *testing.T) {
	service := startAPI(t)
	window := "&from=2026-10-01T00:00:00Z&to=2026-10-08T00:00:00Z"
	for _, query := range []string{
		"bucket=day&to=2026-10-08T00:00:00Z",
		"bucket=day&from=2026-10-01T00:00:00Z&to=next+week",
		"bucket=day&from=2026-10-08T00:00:00Z&to=2026-10-01T00:00:00Z",
		"bucket=minute" + window,
		window[1:],
		"bucket=day&kind=job_dream" + window,
		"bucket=day&time_zone=Mars/Olympus_Mons" + window,
		"bucket=day&time_zone=Local" + window,
		"bucket=hour&from=2026-01-01T00:00:00Z&to=2027-01-01T00:00:00Z",
	} {
		if status, answer := send(t, http.MethodGet, service.url+"/v1/runs/metrics?"+query, ownerToken, ""); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", query, status, answer)
		}
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/runs/metrics?bucket=day"+window, "", ""); status != http.StatusUnauthorized {
		t.Errorf("without a token: %d, want 401", status)
	}
}
