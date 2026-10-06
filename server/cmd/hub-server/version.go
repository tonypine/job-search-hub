package main

import (
	"fmt"
	"io"

	"github.com/tonypine/job-search-hub/server/internal/api"
)

// printVersion prints what GET /v1/version answers: the version, the commit
// and the newest migration this build knows.
func printVersion(w io.Writer) {
	version := api.CurrentVersion()
	fmt.Fprintf(w, "hub-server %s\ncommit %s\nnewest migration %d\n", version.Version, version.Commit, version.NewestMigration)
}
