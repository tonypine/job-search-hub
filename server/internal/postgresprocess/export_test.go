package postgresprocess

import (
	"testing"
	"time"
)

// ShortenStopTimeouts sets how long Stop waits after SIGINT and after
// SIGQUIT, until the test ends.
func ShortenStopTimeouts(t *testing.T, stop, quit time.Duration) {
	savedStop, savedQuit := stopTimeout, quitTimeout
	stopTimeout, quitTimeout = stop, quit
	t.Cleanup(func() { stopTimeout, quitTimeout = savedStop, savedQuit })
}
