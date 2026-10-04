package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheOwnersRolesCountEachMonthOnce(t *testing.T) {
	now := time.Date(2026, time.October, 4, 0, 0, 0, 0, time.UTC)
	roles := []store.ProfileEntry{
		{StartMonth: "2018-01", EndMonth: "2019-12"},
		{StartMonth: "2019-07", EndMonth: "2020-06"},  // overlaps the first by six months
		{StartMonth: "2019-01", EndMonth: "2019-03"},  // inside the first
		{StartMonth: "2015", EndMonth: "2015"},        // a year alone is twelve months
		{StartMonth: "2026-01"},                       // current, runs to October
		{StartMonth: "", EndMonth: "2012-01"},         // no start: left out
		{StartMonth: "sometime", EndMonth: "2013-01"}, // unreadable: left out
		{StartMonth: "2021-05", EndMonth: "2021-01"},  // ends before it starts: left out
	}
	if months := countExperienceMonths(roles, now); months != 24+6+12+10 {
		t.Errorf("months = %d, want %d", months, 24+6+12+10)
	}
	if months := countExperienceMonths(nil, now); months != 0 {
		t.Errorf("no roles: months = %d, want 0", months)
	}
}

func TestExperienceIsAnsweredAgainstTheOwnersYears(t *testing.T) {
	facts := func(years string) map[string]store.JobFactEntry {
		return map[string]store.JobFactEntry{"years_of_experience": {Key: "years_of_experience", Value: json.RawMessage(years), Evidence: "years in the role"}}
	}
	for _, test := range []struct {
		name    string
		years   string
		months  int
		verdict jobfit.Verdict
		answer  string
	}{
		{"met", `5`, 8 * 12, jobfit.VerdictYes, "asks for 5 years; you have 8 years"},
		{"met exactly", `5`, 5 * 12, jobfit.VerdictYes, "asks for 5 years; you have 5 years"},
		{"a year short", `5`, 4*12 + 6, jobfit.VerdictUnclear, "asks for 5 years; you have 4 years"},
		{"further short", `10`, 6 * 12, jobfit.VerdictNo, "asks for 10 years; you have 6 years"},
		{"as text", `"5+ years"`, 3 * 12, jobfit.VerdictNo, "asks for 5 years; you have 3 years"},
		{"one year", `1`, 12, jobfit.VerdictYes, "asks for 1 year; you have 1 year"},
		{"no dated roles", `5`, 0, jobfit.VerdictUnclear, "asks for 5 years; your knowledge base has no confirmed roles with dates"},
		{"not stated", `null`, 8 * 12, jobfit.VerdictUnclear, "the posting doesn't say"},
		{"not stated as text", `"not stated"`, 8 * 12, jobfit.VerdictUnclear, "the posting doesn't say"},
	} {
		answer := answerExperience(facts(test.years), test.months)
		if answer.Name != "Experience" || answer.Verdict != test.verdict || answer.Answer != test.answer || answer.Evidence != "years in the role" {
			t.Errorf("%s: answer = %+v, want %s %q", test.name, answer, test.verdict, test.answer)
		}
	}
	if answer := answerExperience(map[string]store.JobFactEntry{}, 8*12); answer.Verdict != jobfit.VerdictUnclear || answer.Answer != "the posting doesn't say" {
		t.Errorf("unread: answer = %+v", answer)
	}
}

func TestScreenOutAnswersNameTheLanguagesAsked(t *testing.T) {
	details := store.JobDetails{Facts: &store.LabelledJobFacts{Entries: []store.JobFactEntry{
		{Key: "languages", Value: json.RawMessage(`["English","German"]`), Evidence: "fluent German"},
		{Key: "years_of_experience", Value: json.RawMessage(`10`), Evidence: "10+ years"},
	}}}
	answers := map[string]screenOutAnswer{}
	for _, answer := range buildScreenOutAnswers(details, jobfit.Fit{}, 6*12) {
		answers[answer.Name] = answer
	}
	if languages := answers["Languages"]; languages.Verdict != "" || languages.Answer != "English, German" || languages.Evidence != "fluent German" {
		t.Errorf("Languages = %+v", languages)
	}
	if experience := answers["Experience"]; experience.Verdict != jobfit.VerdictNo || experience.Evidence != "10+ years" {
		t.Errorf("Experience = %+v", experience)
	}
	if len(answers) != 6 {
		t.Errorf("answers = %+v, want six", answers)
	}
	none := map[string]screenOutAnswer{}
	for _, answer := range buildScreenOutAnswers(store.JobDetails{}, jobfit.Fit{}, 0) {
		none[answer.Name] = answer
	}
	if languages := none["Languages"]; languages.Answer != "the posting doesn't say" {
		t.Errorf("unread Languages = %+v", languages)
	}
}
