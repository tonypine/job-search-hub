package jobfacts_test

import (
	"encoding/json"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestFindDoubtFlagsTheReadingsAStrongerModelShouldReadAgain(t *testing.T) {
	remote := store.Job{Title: "Senior Frontend Engineer", Location: "Remote"}
	for _, test := range []struct {
		name  string
		job   store.Job
		facts string
		want  string
	}{
		{"unclear", remote, `{"location":{"evidence":"Americas","restriction":"Americas","open_to_brazil":"unclear","reason":"x"}}`, jobfacts.DoubtOpenToBrazilUnclear},
		{"yes with no location", remote, `{"location":{"evidence":"","restriction":"not stated","open_to_brazil":"yes","reason":"x"}}`, jobfacts.DoubtOpenWithoutLocation},
		{"yes with an empty location", remote, `{"location":{"evidence":"","restriction":"","open_to_brazil":"Yes","reason":"x"}}`, jobfacts.DoubtOpenWithoutLocation},
		{"worldwide with a US-only rule", remote,
			`{"location":{"evidence":"Remote, worldwide","restriction":"Worldwide","open_to_brazil":"yes","reason":"x"},
			  "work_authorization":{"evidence":"Must be authorized to work in the US","value":"US work authorization"}}`, jobfacts.DoubtWorldwideButLimited},
		{"worldwide job location with US-only benefits", store.Job{Title: "Engineer", Location: "Anywhere"},
			`{"location":{"evidence":"Choose where you live; benefits for US residents only","restriction":"Choose where you live","open_to_brazil":"yes","reason":"x"}}`,
			jobfacts.DoubtWorldwideButLimited},
		{"flat schema worldwide with a limit", remote, `{"location_restriction":"Global, US citizens only"}`, jobfacts.DoubtWorldwideButLimited},
		{"levels with no stated level", remote,
			`{"location":{"evidence":"LATAM","restriction":"LATAM","open_to_brazil":"yes","reason":"x"},"seniority":{"evidence":"","as_written":"not stated","levels":["senior"]}}`,
			jobfacts.DoubtLevelWithoutMention},
		{"clear yes", remote,
			`{"location":{"evidence":"LATAM","restriction":"LATAM","open_to_brazil":"yes","reason":"x"},"seniority":{"evidence":"Senior","as_written":"Senior","levels":["senior"]}}`, ""},
		{"clear no", remote, `{"location":{"evidence":"US only","restriction":"United States","open_to_brazil":"no","reason":"x"}}`, ""},
		{"worldwide with no limit", remote,
			`{"location":{"evidence":"Work from anywhere","restriction":"Worldwide","open_to_brazil":"yes","reason":"x"},"work_authorization":{"evidence":"","value":"not stated"}}`, ""},
		{"no levels read", remote, `{"seniority":{"evidence":"","as_written":"","levels":[]}}`, ""},
		{"not an object", remote, `"oops"`, ""},
	} {
		if got := jobfacts.FindDoubt(test.job, json.RawMessage(test.facts)); got != test.want {
			t.Errorf("%s: doubt = %q, want %q", test.name, got, test.want)
		}
	}
}
