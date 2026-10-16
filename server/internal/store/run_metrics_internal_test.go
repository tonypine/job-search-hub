package store

import (
	"errors"
	"testing"
	"time"
)

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func TestRunBucketsFollowTheOwnersClock(t *testing.T) {
	newYork := mustLoadLocation(t, "America/New_York")
	kolkata := mustLoadLocation(t, "Asia/Kolkata")
	for _, test := range []struct {
		name     string
		location *time.Location
		from, to time.Time
		bucket   string
		want     []time.Time
	}{
		{
			// Clocks go forward on Sunday 8 March: that day is 23 hours long.
			name: "days across spring forward", location: newYork, bucket: RunBucketDay,
			from: time.Date(2026, 3, 5, 0, 0, 0, 0, newYork), to: time.Date(2026, 3, 12, 0, 0, 0, 0, newYork),
			want: []time.Time{
				time.Date(2026, 3, 5, 0, 0, 0, 0, newYork), time.Date(2026, 3, 6, 0, 0, 0, 0, newYork),
				time.Date(2026, 3, 7, 0, 0, 0, 0, newYork), time.Date(2026, 3, 8, 0, 0, 0, 0, newYork),
				time.Date(2026, 3, 9, 0, 0, 0, 0, newYork), time.Date(2026, 3, 10, 0, 0, 0, 0, newYork),
				time.Date(2026, 3, 11, 0, 0, 0, 0, newYork), time.Date(2026, 3, 12, 0, 0, 0, 0, newYork),
			},
		},
		{
			// Clocks go back at 2:00 on Sunday 1 November: 1:00 comes twice,
			// and each is an hour of its own.
			name: "hours across fall back", location: newYork, bucket: RunBucketHour,
			from: time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC), to: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
			want: []time.Time{
				time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 5, 0, 0, 0, time.UTC),
				time.Date(2026, 11, 1, 6, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 7, 0, 0, 0, time.UTC),
				time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "weeks from Monday, clipped to the window", location: newYork, bucket: RunBucketWeek,
			from: time.Date(2026, 10, 14, 15, 0, 0, 0, newYork), to: time.Date(2026, 10, 28, 9, 0, 0, 0, newYork),
			want: []time.Time{
				time.Date(2026, 10, 14, 15, 0, 0, 0, newYork), time.Date(2026, 10, 19, 0, 0, 0, 0, newYork),
				time.Date(2026, 10, 26, 0, 0, 0, 0, newYork), time.Date(2026, 10, 28, 9, 0, 0, 0, newYork),
			},
		},
		{
			// India is 5:30 ahead of UTC, so its hours start on the half hour.
			name: "hours of a zone on the half hour", location: kolkata, bucket: RunBucketHour,
			from: time.Date(2026, 10, 16, 10, 15, 0, 0, kolkata), to: time.Date(2026, 10, 16, 12, 0, 0, 0, kolkata),
			want: []time.Time{
				time.Date(2026, 10, 16, 10, 15, 0, 0, kolkata), time.Date(2026, 10, 16, 11, 0, 0, 0, kolkata),
				time.Date(2026, 10, 16, 12, 0, 0, 0, kolkata),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			edges, err := getRunBucketEdges(test.from, test.to, test.bucket, test.location)
			if err != nil {
				t.Fatal(err)
			}
			if len(edges) != len(test.want) {
				t.Fatalf("edges = %v; want %v", edges, test.want)
			}
			for index := range edges {
				if !edges[index].Equal(test.want[index]) || edges[index].Location() != test.location {
					t.Errorf("edge %d = %v; want %v", index, edges[index], test.want[index])
				}
			}
		})
	}
	day := time.Date(2026, 3, 8, 0, 0, 0, 0, newYork)
	if edges, _ := getRunBucketEdges(day, day.AddDate(0, 0, 1), RunBucketDay, newYork); len(edges) != 2 || edges[1].Sub(edges[0]) != 23*time.Hour {
		t.Errorf("the day clocks go forward = %v; want one bucket of 23 hours", edges)
	}
}

func TestRunBucketsRefuseWhatAChartCantDraw(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := getRunBucketEdges(from, from.AddDate(0, 0, 90), RunBucketHour, time.UTC); err != nil {
		t.Errorf("90 days by the hour: %v", err)
	}
	if _, err := getRunBucketEdges(from, from.AddDate(1, 0, 0), RunBucketHour, time.UTC); !errors.Is(err, ErrTooManyRunBuckets) {
		t.Errorf("a year by the hour: %v", err)
	}
	if _, err := getRunBucketEdges(from, from.AddDate(0, 0, 1), "minute", time.UTC); !errors.Is(err, ErrInvalidRunBucket) {
		t.Errorf("by the minute: %v", err)
	}
}
