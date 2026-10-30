package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// The sizes of a bucket of run metrics: an hour, or one of the owner's days
// or weeks, which start on Monday.
const (
	RunBucketHour = "hour"
	RunBucketDay  = "day"
	RunBucketWeek = "week"
)

// maximumRunBuckets keeps a window to what a chart can draw: 90 days by the
// hour fit, a year by the hour doesn't.
const maximumRunBuckets = 5000

var (
	// ErrInvalidRunWindow is a window that ends before it starts.
	ErrInvalidRunWindow = errors.New("a window's from must be before its to")
	// ErrInvalidRunBucket is a bucket that isn't an hour, a day or a week.
	ErrInvalidRunBucket = errors.New("a bucket is hour, day or week")
	// ErrTooManyRunBuckets is a window split into more buckets than a chart
	// can draw.
	ErrTooManyRunBuckets = errors.New("the window has too many buckets: pick a shorter window or a bigger bucket")
)

// RunMetricsFilter is the runs to count: of one kind, or of every kind when
// Kind is empty, started from From until before To, split into buckets of
// Bucket in Location, the owner's time zone.
type RunMetricsFilter struct {
	Kind     string
	From     time.Time
	To       time.Time
	Bucket   string
	Location *time.Location
}

// RunTotals counts the runs of a window, from both records.
type RunTotals struct {
	From         time.Time `json:"from"`
	To           time.Time `json:"to"`
	Runs         int       `json:"runs"`
	Failed       int       `json:"failed"`
	Invalid      int       `json:"invalid"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	// AverageDurationMS is over the runs that finished, nil when none did.
	AverageDurationMS *int `json:"average_duration_ms"`
	// CostUSD is what the runs that report a cost cost, as text so the
	// decimal is never rounded: Claude agent runs, and model calls on a
	// provider that charges for them, such as OpenRouter.
	CostUSD string `json:"cost_usd"`
	// ClaudePlanRuns are the model calls on the owner's Claude plan, which
	// cost nothing more and count as zero in CostUSD.
	ClaudePlanRuns int `json:"claude_plan_runs"`
}

// RunKindCount counts one kind's runs in a bucket.
type RunKindCount struct {
	Kind         string `json:"kind"`
	Runs         int    `json:"runs"`
	Failed       int    `json:"failed"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// RunBucket counts the runs started from Start until before End, in all and
// by kind. Kinds lists only the kinds that ran, and is empty when none did.
type RunBucket struct {
	Start        time.Time      `json:"start"`
	End          time.Time      `json:"end"`
	Runs         int            `json:"runs"`
	Failed       int            `json:"failed"`
	InputTokens  int64          `json:"input_tokens"`
	OutputTokens int64          `json:"output_tokens"`
	Kinds        []RunKindCount `json:"kinds"`
}

// RunMetrics counts a window's runs, the window just before it for
// comparison, and every bucket of the window, the empty ones included.
type RunMetrics struct {
	Kind     string      `json:"kind"`
	Bucket   string      `json:"bucket"`
	TimeZone string      `json:"time_zone"`
	Totals   RunTotals   `json:"totals"`
	Previous RunTotals   `json:"previous"`
	Buckets  []RunBucket `json:"buckets"`
}

// getRunBucketEdges returns where each bucket of the window starts, then the
// window's end. Buckets start on the owner's hours, midnights or Mondays, so
// a day is 23 or 25 hours long across a daylight-saving change, and the first
// and last are clipped to the window.
func getRunBucketEdges(from, to time.Time, bucket string, location *time.Location) ([]time.Time, error) {
	local := from.In(location)
	var getStart func(index int) time.Time
	switch bucket {
	case RunBucketHour:
		// An hour is a real hour: the hour a clock repeats in autumn is two
		// buckets, and the one it skips in spring none.
		hour := local.Add(-time.Duration(local.Minute())*time.Minute - time.Duration(local.Second())*time.Second -
			time.Duration(local.Nanosecond()))
		getStart = func(index int) time.Time { return hour.Add(time.Duration(index) * time.Hour) }
	case RunBucketDay:
		getStart = func(index int) time.Time {
			return time.Date(local.Year(), local.Month(), local.Day()+index, 0, 0, 0, 0, location)
		}
	case RunBucketWeek:
		sinceMonday := (int(local.Weekday()) + 6) % 7
		getStart = func(index int) time.Time {
			return time.Date(local.Year(), local.Month(), local.Day()-sinceMonday+7*index, 0, 0, 0, 0, location)
		}
	default:
		return nil, ErrInvalidRunBucket
	}
	edges := []time.Time{from.In(location)}
	for index := 1; ; index++ {
		start := getStart(index)
		if !start.Before(to) {
			break
		}
		if !start.After(edges[len(edges)-1]) {
			continue
		}
		if len(edges) == maximumRunBuckets {
			return nil, ErrTooManyRunBuckets
		}
		edges = append(edges, start)
	}
	return append(edges, to.In(location)), nil
}

// runMetricsRuns merges both records' runs started from $1 until before $2,
// of kind $3 or of every kind when it's empty. A model call through the
// Claude CLI is on the owner's Claude plan, so what Claude reckons it would
// cost on its API counts as nothing; a model call that a provider charged
// for, and an agent run, report their cost.
const runMetricsRuns = `
	WITH runs AS (
		SELECT kind, started_at, outcome, prompt_tokens, completion_tokens, duration_ms,
			CASE WHEN base_url <> 'claude-cli' THEN cost_usd END AS cost_usd, base_url = 'claude-cli' AS on_claude_plan
		FROM task_runs
		WHERE started_at >= $1 AND started_at < $2 AND ($3 = '' OR kind = $3)
		UNION ALL
		SELECT kind, started_at, status, prompt_tokens, completion_tokens, round(extract(epoch FROM finished_at - started_at) * 1000)::bigint, cost_usd_estimate,
			false
		FROM agent_runs
		WHERE started_at >= $1 AND started_at < $2 AND ($3 = '' OR kind = $3)
	)`

// GetRunMetrics counts the filter's runs from both records: the window's
// totals, the totals of the window of the same length just before it, and
// each bucket of the window by kind.
func (s *Store) GetRunMetrics(ctx context.Context, filter RunMetricsFilter) (RunMetrics, error) {
	location := filter.Location
	if location == nil {
		location = time.UTC
	}
	if !filter.From.Before(filter.To) {
		return RunMetrics{}, ErrInvalidRunWindow
	}
	edges, err := getRunBucketEdges(filter.From, filter.To, filter.Bucket, location)
	if err != nil {
		return RunMetrics{}, err
	}
	from, to := edges[0], edges[len(edges)-1]
	previousFrom := from.Add(-to.Sub(from))
	metrics := RunMetrics{
		Kind: filter.Kind, Bucket: filter.Bucket, TimeZone: location.String(),
		Totals:   RunTotals{From: from, To: to, CostUSD: "0"},
		Previous: RunTotals{From: previousFrom, To: from, CostUSD: "0"},
		Buckets:  make([]RunBucket, len(edges)-1),
	}

	rows, err := s.pool.Query(ctx, runMetricsRuns+`
		SELECT started_at >= $4, count(*), count(*) FILTER (WHERE outcome = 'failed'), count(*) FILTER (WHERE outcome = 'invalid'),
			coalesce(sum(prompt_tokens), 0), coalesce(sum(completion_tokens), 0), round(avg(duration_ms))::bigint,
			coalesce(sum(cost_usd), 0)::text, count(*) FILTER (WHERE on_claude_plan)
		FROM runs
		GROUP BY 1`, previousFrom, to, filter.Kind, from)
	if err != nil {
		return RunMetrics{}, err
	}
	if _, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct{}, error) {
		var current bool
		var totals RunTotals
		err := row.Scan(&current, &totals.Runs, &totals.Failed, &totals.Invalid, &totals.InputTokens, &totals.OutputTokens,
			&totals.AverageDurationMS, &totals.CostUSD, &totals.ClaudePlanRuns)
		if err != nil {
			return struct{}{}, err
		}
		window := &metrics.Previous
		if current {
			window = &metrics.Totals
		}
		totals.From, totals.To = window.From, window.To
		*window = totals
		return struct{}{}, nil
	}); err != nil {
		return RunMetrics{}, err
	}

	// A run's bucket is the number of bucket starts at or before it, from 1.
	for index := range metrics.Buckets {
		metrics.Buckets[index] = RunBucket{Start: edges[index], End: edges[index+1], Kinds: []RunKindCount{}}
	}
	rows, err = s.pool.Query(ctx, runMetricsRuns+`
		SELECT width_bucket(started_at, $4::timestamptz[]), kind, count(*), count(*) FILTER (WHERE outcome = 'failed'),
			sum(prompt_tokens), sum(completion_tokens)
		FROM runs
		GROUP BY 1, 2
		ORDER BY 1, 2`, from, to, filter.Kind, edges[:len(edges)-1])
	if err != nil {
		return RunMetrics{}, err
	}
	var position int
	var count RunKindCount
	if _, err := pgx.ForEachRow(rows, []any{&position, &count.Kind, &count.Runs, &count.Failed, &count.InputTokens, &count.OutputTokens},
		func() error {
			bucket := &metrics.Buckets[position-1]
			bucket.Runs += count.Runs
			bucket.Failed += count.Failed
			bucket.InputTokens += count.InputTokens
			bucket.OutputTokens += count.OutputTokens
			bucket.Kinds = append(bucket.Kinds, count)
			return nil
		}); err != nil {
		return RunMetrics{}, err
	}
	return metrics, nil
}
