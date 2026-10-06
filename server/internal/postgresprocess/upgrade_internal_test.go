package postgresprocess

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRowCountsDifferWhenATableLostRowsOrIsMissingOnEitherSide(t *testing.T) {
	before := map[string]int64{"public.companies": 50, "public.notes": 150, `public."Empty Table"`: 0}
	if err := compareRowCounts(before, map[string]int64{"public.companies": 50, "public.notes": 150, `public."Empty Table"`: 0}); err != nil {
		t.Fatalf("equal counts: %v", err)
	}

	err := compareRowCounts(before, map[string]int64{"public.companies": 49, "public.notes": 150, "public.extra": 3})
	if err == nil {
		t.Fatal("different counts pass")
	}
	for _, want := range []string{
		"public.companies: 50 rows before, 49 after",
		`public."Empty Table": 0 rows before, missing after`,
		"public.extra: missing before, 3 rows after",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error doesn't say %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "public.notes") {
		t.Errorf("the error names a table that matched: %v", err)
	}
}

func TestAClusterMovedIntoPlaceStaysThereWhenItsFolderDoesNotSync(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a folder it can't read")
	}
	dir := t.TempDir()
	partial := filepath.Join(dir, "18.partial")
	if err := os.Mkdir(partial, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partial, "PG_VERSION"), []byte("18\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Without read permission the folder can still be renamed in, but not
	// opened to sync.
	if err := os.Chmod(dir, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if syncDir(dir) == nil {
		t.Fatal("the folder syncs; the test can't make it fail")
	}

	if err := moveIntoPlace(partial, filepath.Join(dir, "18")); err != nil {
		t.Fatalf("the move fails after the rename, so Start would fall back to the old cluster: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "18", "PG_VERSION")); err != nil {
		t.Fatalf("the cluster isn't in place: %v", err)
	}
	if _, err := os.Stat(partial); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the partial cluster is still there: %v", err)
	}
}
