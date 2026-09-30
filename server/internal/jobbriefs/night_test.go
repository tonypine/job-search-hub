package jobbriefs

import (
	"testing"
	"time"
)

func TestTheNightsFullBriefsStartOnlyAtNightAndOnce(t *testing.T) {
	at := func(clock string) time.Time {
		moment, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-30 "+clock, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		return moment
	}
	for _, test := range []struct {
		clock, lastNight string
		want             bool
	}{
		{"01:59", "", false},
		{"02:00", "", true},
		{"05:59", "2026-09-29", true},
		{"03:00", "2026-09-30", false},
		{"06:00", "", false},
		{"20:30", "", false},
	} {
		if got := isNightDue(at(test.clock), test.lastNight); got != test.want {
			t.Errorf("at %s, last night %q: due = %v, want %v", test.clock, test.lastNight, got, test.want)
		}
	}
}
