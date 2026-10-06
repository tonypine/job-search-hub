package postgresprocess

import (
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
