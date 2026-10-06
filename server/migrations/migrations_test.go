package migrations

import (
	"fmt"
	"io/fs"
	"testing"
)

func TestNewestIsTheHighestNumberedMigration(t *testing.T) {
	names, err := fs.Glob(Files, "*.sql")
	if err != nil || len(names) == 0 {
		t.Fatalf("migrations: %v %v", names, err)
	}
	// The files sort by their zero-padded number.
	last := names[len(names)-1]
	if got := fmt.Sprintf("%04d", Newest()); got != last[:4] {
		t.Fatalf("Newest() = %s, want the number of %s", got, last)
	}
}
