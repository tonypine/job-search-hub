package comparisons

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestAgreementIgnoresQuotesCaseAndListOrder(t *testing.T) {
	key, local, broken := store.ComparisonStack{ID: uuid.New()}, store.ComparisonStack{ID: uuid.New()}, store.ComparisonStack{ID: uuid.New()}
	first, second := uuid.New(), uuid.New()
	record := store.ComparisonRecord{
		Comparison: store.Comparison{Stacks: []store.ComparisonStack{key, local, broken}, JobIDs: []uuid.UUID{first, second}},
		Answers: []store.ComparisonAnswer{
			{StackID: key.ID, JobID: first, Answer: json.RawMessage(`{"technologies":{"evidence":"React, Go","value":["React","Go"]},"location":{"open_to_brazil":"yes","reason":"Says LatAm."}}`)},
			{StackID: local.ID, JobID: first, Answer: json.RawMessage(`{"technologies":{"evidence":"Go and React","value":["go","React "]},"location":{"open_to_brazil":"unclear","reason":"Remote."}}`)},
			{StackID: key.ID, JobID: second, Answer: json.RawMessage(`{"technologies":{"value":[]},"location":{"open_to_brazil":"no"}}`)},
			{StackID: local.ID, JobID: second, Answer: json.RawMessage(`{"technologies":{"value":[]},"location":{"open_to_brazil":"no"}}`)},
			{StackID: broken.ID, JobID: first, Error: "timed out"},
		},
		Verdicts: []store.ComparisonVerdict{{StackID: local.ID, JobID: first, Field: "location.open_to_brazil", Verdict: "wrong"}},
	}

	summary := Summarize(record)
	if len(summary.Fields) != 2 || summary.Fields[0] != "location.open_to_brazil" || summary.Fields[1] != "technologies.value" {
		t.Fatalf("fields = %v, want the leaves without quotes", summary.Fields)
	}
	localSummary := summary.Stacks[1]
	if localSummary.Answered != 2 || localSummary.Fields[1] != (FieldScore{Field: "technologies.value", Agreed: 2, Compared: 2}) {
		t.Errorf("technologies = %+v", localSummary.Fields[1])
	}
	if localSummary.Fields[0] != (FieldScore{Field: "location.open_to_brazil", Agreed: 1, Compared: 2, Wrong: 1}) {
		t.Errorf("location = %+v", localSummary.Fields[0])
	}
	if summary.Stacks[0].Fields[0].Compared != 0 {
		t.Errorf("the first stack was compared with itself: %+v", summary.Stacks[0].Fields)
	}
	if brokenSummary := summary.Stacks[2]; brokenSummary.Answered != 0 || brokenSummary.Failed != 1 || brokenSummary.Fields[0].Compared != 0 {
		t.Errorf("the failing stack = %+v", brokenSummary)
	}
}
