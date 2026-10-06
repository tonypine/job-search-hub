package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
	"github.com/tonypine/job-search-hub/server/migrations"
)

func TestVersionPrintsTheVersionCommitAndNewestMigration(t *testing.T) {
	var out bytes.Buffer
	printVersion(&out)
	want := fmt.Sprintf("hub-server %s\ncommit %s\nnewest migration %d\n", buildinfo.Version(), buildinfo.Commit(), migrations.Newest())
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
