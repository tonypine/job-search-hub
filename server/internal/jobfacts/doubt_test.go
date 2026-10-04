package jobfacts_test

import (
	"encoding/json"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
)

func TestFindDoubtFlagsTheReadingsAStrongerModelShouldReadAgain(t *testing.T) {
	for _, test := range []struct {
		name  string
		facts string
		want  string
	}{
		{"unclear", `{"location":{"evidence":"Americas","restriction":"Americas","open_to_brazil":"unclear","reason":"x"}}`, jobfacts.DoubtOpenToBrazilUnclear},
		{"yes with no location", `{"location":{"evidence":"","restriction":"not stated","open_to_brazil":"yes","reason":"x"}}`, jobfacts.DoubtOpenWithoutLocation},
		{"yes with an empty location", `{"location":{"evidence":"","restriction":"","open_to_brazil":"Yes","reason":"x"}}`, jobfacts.DoubtOpenWithoutLocation},
		{"worldwide with a US-only rule",
			`{"location":{"evidence":"Remote, worldwide","restriction":"Worldwide, must be authorized to work in the US","open_to_brazil":"yes","reason":"x"}}`,
			jobfacts.DoubtWorldwideButLimited},
		{"flat schema worldwide with a limit", `{"location_restriction":"Global, US citizens only"}`, jobfacts.DoubtWorldwideButLimited},
		{"levels with no stated level",
			`{"location":{"evidence":"LATAM","restriction":"LATAM","open_to_brazil":"yes","reason":"x"},"seniority":{"evidence":"","as_written":"not stated","levels":["senior"]}}`,
			jobfacts.DoubtLevelWithoutMention},
		{"clear yes",
			`{"location":{"evidence":"LATAM","restriction":"LATAM","open_to_brazil":"yes","reason":"x"},"seniority":{"evidence":"Senior","as_written":"Senior","levels":["senior"]}}`, ""},
		{"clear no", `{"location":{"evidence":"US only","restriction":"United States","open_to_brazil":"no","reason":"x"}}`, ""},
		{"worldwide with no limit",
			`{"location":{"evidence":"Work from anywhere","restriction":"Worldwide","open_to_brazil":"yes","reason":"x"},"work_authorization":{"evidence":"","value":"not stated"}}`, ""},
		{"worldwide with a limit only in another fact",
			`{"location":{"evidence":"Remote, anywhere","restriction":"Anywhere","open_to_brazil":"yes","reason":"x"},"visa_sponsorship":{"evidence":"No sponsorship","value":"No sponsorship"}}`, ""},
		{"worldwide with a limit only in the evidence",
			`{"location":{"evidence":"Remote only","restriction":"Worldwide","open_to_brazil":"yes","reason":"x"}}`, ""},
		{"levels with an empty level as written",
			`{"location":{"evidence":"LATAM","restriction":"LATAM","open_to_brazil":"yes","reason":"x"},"seniority":{"evidence":"","as_written":"","levels":["senior"]}}`, ""},
		{"no levels read", `{"seniority":{"evidence":"","as_written":"","levels":[]}}`, ""},
		{"not an object", `"oops"`, ""},
	} {
		if got := jobfacts.FindDoubt(json.RawMessage(test.facts)); got != test.want {
			t.Errorf("%s: doubt = %q, want %q", test.name, got, test.want)
		}
	}
}
