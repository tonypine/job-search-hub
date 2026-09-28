package api

import (
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
)

func TestADraftNamesGoodFitsFirstAndNeverPoorOnes(t *testing.T) {
	picked := pickOpeningsForDraft([]opening{
		{Title: "Unclear", Fit: jobfit.LevelUnclear}, {Title: "Poor", Fit: jobfit.LevelPoor}, {Title: "Good", Fit: jobfit.LevelGood},
	})
	if len(picked) != 2 || picked[0].Title != "Good" || picked[1].Title != "Unclear" {
		t.Fatalf("picked = %+v", picked)
	}
}
