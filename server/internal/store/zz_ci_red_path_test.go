package store_test

import (
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestCIRedPathFailsOnPurpose(t *testing.T) {
	testdatabase.New(t)
	t.Fatal("intentional failure: TP-399 red-path check, this PR is closed unmerged")
}
