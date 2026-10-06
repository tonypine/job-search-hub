package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// databaseWaitLimit is how long the server waits at startup for a Postgres
// it doesn't run. At login, launchd starts the server before Docker Desktop
// has Postgres up.
const databaseWaitLimit = 3 * time.Minute

type pinger interface {
	Ping(ctx context.Context) error
}

// databaseWait is how waitForDatabase paces its attempts: the first pause,
// doubled after each failure up to the longest, until the limit has passed.
type databaseWait struct {
	limit        time.Duration
	firstPause   time.Duration
	longestPause time.Duration
}

var startupDatabaseWait = databaseWait{limit: databaseWaitLimit, firstPause: 500 * time.Millisecond, longestPause: 10 * time.Second}

// waitForDatabase returns once the database answers a ping, or an error with
// the last failure once the wait's limit has passed or ctx is done.
func waitForDatabase(ctx context.Context, database pinger, wait databaseWait) error {
	deadline := time.Now().Add(wait.limit)
	pause := wait.firstPause
	for attempt := 1; ; attempt++ {
		err := database.Ping(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().Add(pause).After(deadline) {
			return fmt.Errorf("the database didn't answer within %s: %w", wait.limit, err)
		}
		slog.Info("waiting for the database", "attempt", attempt, "error", err.Error())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pause):
		}
		pause = min(pause*2, wait.longestPause)
	}
}
