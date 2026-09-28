package jobfit_test

import (
	"encoding/json"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

var criteria = store.JobCriteria{
	Technologies:            []string{"TypeScript", "React", "Node.js"},
	SeniorityLevels:         []string{"Senior", "Staff"},
	HomeCountry:             "Brazil",
	EligibleLocationTerms:   []string{"Brazil", "LATAM", "Americas", "Worldwide"},
	IneligibleLocationTerms: []string{"must reside in the US", "US only"},
	RefuseHourlyWork:        true,
}

func facts(t *testing.T, values map[string]any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func findCheck(t *testing.T, fit jobfit.Fit, name string) jobfit.Check {
	t.Helper()
	for _, check := range fit.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("no %q check in %+v", name, fit.Checks)
	return jobfit.Check{}
}

func TestWhereTheyHire(t *testing.T) {
	for _, test := range []struct {
		name        string
		job         store.Job
		restriction string
		want        jobfit.Verdict
	}{
		{"an eligible region", store.Job{Location: "Americas"}, "not stated", jobfit.VerdictYes},
		{"the home country among other locations", store.Job{Location: "5 countries", BoardFacts: store.BoardFacts{OtherLocations: []string{"Chile", "Brazil"}}}, "", jobfit.VerdictYes},
		{"a residency rule beside an eligible region", store.Job{Location: "Americas"}, "Remote, but you must reside in the US", jobfit.VerdictNo},
		{"another country only", store.Job{Location: "United States (Remote)"}, "not stated", jobfit.VerdictNo},
		{"nothing said", store.Job{Location: "Remote"}, "not stated", jobfit.VerdictUnclear},
		{"a term inside another word", store.Job{Location: "Remote"}, "Across the Americas; our focus only matters", jobfit.VerdictYes},
	} {
		fit := jobfit.Judge(test.job, facts(t, map[string]any{"location_restriction": test.restriction}), criteria)
		if check := findCheck(t, fit, "Where they hire"); check.Verdict != test.want {
			t.Errorf("%s: %+v, want %s", test.name, check, test.want)
		}
	}
}

func TestStackAndLevel(t *testing.T) {
	for _, test := range []struct {
		name      string
		title     string
		read      map[string]any
		wantStack jobfit.Verdict
		wantLevel jobfit.Verdict
	}{
		{"an overlapping stack", "Senior Engineer", map[string]any{"technologies": []string{"React.js", "GraphQL"}, "seniority": "not stated"}, jobfit.VerdictYes, jobfit.VerdictYes},
		{"the stack in the title", "Staff React Engineer", map[string]any{"technologies": []string{}}, jobfit.VerdictYes, jobfit.VerdictYes},
		{"another stack and level", "Backend Engineer", map[string]any{"technologies": []string{"Java", "Spring"}, "seniority": "Mid-level"}, jobfit.VerdictNo, jobfit.VerdictNo},
		{"nothing read yet", "Engineer", nil, jobfit.VerdictUnclear, jobfit.VerdictUnclear},
		{"a level with an accent", "Desenvolvedor React Sênior", map[string]any{"seniority": "Sênior"}, jobfit.VerdictYes, jobfit.VerdictYes},
	} {
		var raw json.RawMessage
		if test.read != nil {
			raw = facts(t, test.read)
		}
		fit := jobfit.Judge(store.Job{Title: test.title, Location: "Americas"}, raw, criteria)
		if stack := findCheck(t, fit, "Stack"); stack.Verdict != test.wantStack {
			t.Errorf("%s stack: %+v, want %s", test.name, stack, test.wantStack)
		}
		if level := findCheck(t, fit, "Level"); level.Verdict != test.wantLevel {
			t.Errorf("%s level: %+v, want %s", test.name, level, test.wantLevel)
		}
	}
}

func TestPay(t *testing.T) {
	yearly := &store.Pay{Ranges: []store.PayRange{{Min: 100000, Max: 150000, Currency: "USD", Interval: "year"}}}
	monthly := &store.Pay{Ranges: []store.PayRange{{Min: 8000, Max: 10000, Currency: "USD", Interval: "month"}}}
	hourly := &store.Pay{Ranges: []store.PayRange{{Min: 50, Max: 70, Currency: "USD", Interval: "hour"}}}
	withFloor := criteria
	withFloor.MinimumYearlyPay = &store.MinimumYearlyPay{Amount: 120000, Currency: "USD"}

	if fit := jobfit.Judge(store.Job{BoardFacts: store.BoardFacts{Pay: yearly}}, nil, criteria); len(fit.Checks) != 3 {
		t.Errorf("without a floor or hourly pay there is no pay check: %+v", fit.Checks)
	}
	for _, test := range []struct {
		name string
		pay  *store.Pay
		text string
		want jobfit.Verdict
	}{
		{"above the floor", yearly, "", jobfit.VerdictYes},
		{"monthly, above the floor over a year", monthly, "", jobfit.VerdictYes},
		{"hourly", hourly, "", jobfit.VerdictNo},
		{"hourly in the text", nil, "$60 per hour", jobfit.VerdictNo},
		{"no pay published", nil, "not stated", jobfit.VerdictUnclear},
	} {
		fit := jobfit.Judge(store.Job{BoardFacts: store.BoardFacts{Pay: test.pay}}, facts(t, map[string]any{"pay_in_text": test.text}), withFloor)
		if check := findCheck(t, fit, "Pay"); check.Verdict != test.want {
			t.Errorf("%s: %+v, want %s", test.name, check, test.want)
		}
	}
	lowFloor := withFloor
	lowFloor.MinimumYearlyPay = &store.MinimumYearlyPay{Amount: 200000, Currency: "USD"}
	if check := findCheck(t, jobfit.Judge(store.Job{BoardFacts: store.BoardFacts{Pay: yearly}}, nil, lowFloor), "Pay"); check.Verdict != jobfit.VerdictNo {
		t.Errorf("below the floor: %+v", check)
	}
}

func TestTheFitLevel(t *testing.T) {
	good := jobfit.Judge(store.Job{Title: "Senior Engineer", Location: "LATAM"}, facts(t, map[string]any{"technologies": []string{"TypeScript"}}), criteria)
	unclear := jobfit.Judge(store.Job{Title: "Senior Engineer", Location: "Remote"}, facts(t, map[string]any{"technologies": []string{"TypeScript"}}), criteria)
	poor := jobfit.Judge(store.Job{Title: "Senior Engineer", Location: "LATAM"}, facts(t, map[string]any{"technologies": []string{"Java"}}), criteria)

	if good.Level != jobfit.LevelGood || unclear.Level != jobfit.LevelUnclear || poor.Level != jobfit.LevelPoor {
		t.Fatalf("levels = %s, %s, %s", good.Level, unclear.Level, poor.Level)
	}
}
