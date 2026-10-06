package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
)

func TestVersionPrintsTheHubsVersionWithoutAConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if want := "hub " + buildinfo.Version() + "\n"; stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}
