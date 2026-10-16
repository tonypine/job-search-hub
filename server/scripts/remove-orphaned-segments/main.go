// Command remove-orphaned-segments removes the System V shared-memory
// segments killed Postgres clusters left behind, as the server does before it
// starts its own. test-with-postgres.sh runs it before and after the tests,
// whose throwaway clusters would otherwise use up macOS's 32.
package main

import (
	"fmt"
	"os"

	"github.com/tonypine/job-search-hub/server/internal/postgresprocess"
)

func main() {
	removed, err := postgresprocess.RemoveOrphanedSegments()
	if removed > 0 {
		fmt.Printf("Removed %d shared-memory segments killed Postgres clusters left behind.\n", removed)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
