package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type scriptedPinger struct {
	failures int
	pings    int
}

func (p *scriptedPinger) Ping(context.Context) error {
	p.pings++
	if p.pings <= p.failures {
		return errors.New("connection refused")
	}
	return nil
}

var quickWait = databaseWait{limit: time.Second, firstPause: time.Millisecond, longestPause: 4 * time.Millisecond}

func TestWaitForDatabaseReturnsOnceTheDatabaseAnswers(t *testing.T) {
	database := &scriptedPinger{failures: 2}

	if err := waitForDatabase(context.Background(), database, quickWait); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if database.pings != 3 {
		t.Errorf("pinged %d times, want 3", database.pings)
	}
}

func TestWaitForDatabaseGivesUpAfterTheLimitWithTheLastError(t *testing.T) {
	database := &scriptedPinger{failures: 1_000_000}
	wait := databaseWait{limit: 20 * time.Millisecond, firstPause: time.Millisecond, longestPause: 2 * time.Millisecond}

	err := waitForDatabase(context.Background(), database, wait)
	if err == nil || !strings.Contains(err.Error(), "didn't answer within 20ms") || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("error = %v, want the limit and the last failure", err)
	}
}

func TestWaitForDatabaseStopsWhenCancelled(t *testing.T) {
	database := &scriptedPinger{failures: 1_000_000}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := waitForDatabase(ctx, database, startupDatabaseWait); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
