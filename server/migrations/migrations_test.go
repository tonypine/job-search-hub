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

func TestNoTwoMigrationsShareANumber(t *testing.T) {
	names, err := fs.Glob(Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Goose refuses to run at all when two migrations share a version, which
	// happens when two branches each take the next number.
	seen := map[string]string{}
	for _, name := range names {
		if earlier, ok := seen[name[:4]]; ok {
			t.Errorf("%s and %s share migration number %s; renumber the newer one", earlier, name, name[:4])
		}
		seen[name[:4]] = name
	}
}
