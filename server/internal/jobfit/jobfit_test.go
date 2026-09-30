package jobfit_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

var criteria = store.JobCriteria{
	Roles:                   []string{"Senior Front-End Engineer", "Senior Full-Stack Engineer", "Product Engineer", "AI Product Engineer"},
	ExcludedRoleTerms:       []string{"Sales", "Manager", "Analyst", "Designer"},
	Technologies:            []string{"TypeScript", "React", "Node.js"},
	SeniorityLevels:         []string{"Senior", "Staff"},
	HomeCountry:             "Brazil",
	EligibleLocationTerms:   []string{"Brazil", "LATAM", "Americas", "Worldwide"},
	IneligibleLocationTerms: []string{"must reside in the US", "US only"},
	RefuseHourlyWork:        true,
	WorkableTimezoneTerms:   []string{"US", "EDT", "EST", "your local time zone"},
	UnworkableTimezoneTerms: []string{"APAC", "Bangkok"},
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
		fit := jobfit.Judge(test.job, facts(t, map[string]any{"location_restriction": test.restriction}), criteria, rates)
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
		fit := jobfit.Judge(store.Job{Title: test.title, Location: "Americas"}, raw, criteria, rates)
		if stack := findCheck(t, fit, "Stack"); stack.Verdict != test.wantStack {
			t.Errorf("%s stack: %+v, want %s", test.name, stack, test.wantStack)
		}
		if level := findCheck(t, fit, "Level"); level.Verdict != test.wantLevel {
			t.Errorf("%s level: %+v, want %s", test.name, level, test.wantLevel)
		}
	}
}

func TestAListingWithoutTextSaysSoInsteadOfWaitingForFacts(t *testing.T) {
	waiting := findCheck(t, jobfit.Judge(store.Job{Title: "Engineer", Description: "Build things."}, nil, criteria, rates), "Stack")
	textless := findCheck(t, jobfit.Judge(store.Job{Title: "Engineer"}, nil, criteria, rates), "Stack")
	if waiting.Reason != "no technologies read yet" || textless.Reason != "the listing has no text to read" {
		t.Errorf("with text: %q; without: %q", waiting.Reason, textless.Reason)
	}
}

var rates = jobfit.ExchangeRates{Base: "BRL", PerBase: map[string]float64{"USD": 0.2, "CAD": 0.27}}

var takeHome = store.TakeHome{
	Currency: "BRL", MinimumMonthly: 16000, TargetMonthly: 44000,
	CLT: store.HiringTakeHome{Share: 0.73, PaymentsPerYear: 13.33}, PJ: store.HiringTakeHome{Share: 0.82, PaymentsPerYear: 12},
	ForeignContractor: store.HiringTakeHome{Share: 0.84, PaymentsPerYear: 12},
}

func pay(maximum float64, currency, interval string) *store.Pay {
	return &store.Pay{Ranges: []store.PayRange{{Min: maximum * 0.8, Max: maximum, Currency: currency, Interval: interval}}}
}

func TestPayIsJudgedByEstimatedTakeHome(t *testing.T) {
	if fit := jobfit.Judge(store.Job{BoardFacts: store.BoardFacts{Pay: pay(25000, "BRL", "month")}}, nil, criteria, rates); len(fit.Checks) != 4 {
		t.Errorf("without a take-home or hourly pay there is no pay check: %+v", fit.Checks)
	}
	withTakeHome := criteria
	withTakeHome.TakeHome = &takeHome

	for _, test := range []struct {
		name     string
		pay      *store.Pay
		contract string
		want     jobfit.Verdict
		reason   string
	}{
		{"CLT in reais", pay(25000, "BRL", "month"), "CLT", jobfit.VerdictYes, "about BRL 20.3k a month take-home, 46% of the target"},
		{"a foreign contract in dollars", pay(60000, "USD", "year"), "Contractor", jobfit.VerdictYes, "about BRL 21.0k a month take-home, 48% of the target"},
		{"PJ under the minimum", pay(12000, "BRL", "month"), "PJ", jobfit.VerdictNo, "about BRL 9.8k a month take-home, under the BRL 16.0k minimum"},
		{"an unstated contract across the minimum", pay(46560, "USD", "year"), "not stated", jobfit.VerdictUnclear, ""},
		{"an unstated period across the minimum", pay(20000, "USD", ""), "Contractor", jobfit.VerdictUnclear, ""},
		{"an unstated period above it both ways", pay(150000, "USD", ""), "Contractor", jobfit.VerdictYes, "at least BRL 52.5k a month take-home, 119% of the target"},
		{"an unstated contract under it both ways", pay(10000, "BRL", "month"), "not stated", jobfit.VerdictNo, "at most BRL 8.2k a month take-home, under the BRL 16.0k minimum"},
		{"hourly, refused", pay(70, "USD", "hour"), "Contractor", jobfit.VerdictNo, "paid by the hour"},
		{"a currency without a rate", pay(900000, "ARS", "month"), "Contractor", jobfit.VerdictUnclear, "no exchange rate from ARS to BRL"},
	} {
		job := store.Job{BoardFacts: store.BoardFacts{Pay: test.pay}}
		check := findCheck(t, jobfit.Judge(job, facts(t, map[string]any{"contract_type": test.contract}), withTakeHome, rates), "Pay")
		if check.Verdict != test.want || (test.reason != "" && check.Reason != test.reason) {
			t.Errorf("%s: %+v, want %s %q", test.name, check, test.want, test.reason)
		}
	}
}

func TestUnpublishedPayIsLeftOutOfTheFit(t *testing.T) {
	withTakeHome := criteria
	withTakeHome.TakeHome = &takeHome

	for _, check := range jobfit.Judge(store.Job{Title: "Senior Frontend Engineer"}, nil, withTakeHome, rates).Checks {
		if check.Name == "Pay" {
			t.Fatalf("a posting without pay got %+v", check)
		}
	}
	findCheck(t, jobfit.Judge(store.Job{BoardFacts: store.BoardFacts{Pay: pay(25000, "BRL", "month")}}, nil, withTakeHome, rates), "Pay")
}

func TestAContractorPostingWithoutACurrencyRateStaysUnclear(t *testing.T) {
	withTakeHome := criteria
	withTakeHome.TakeHome = &takeHome
	job := store.Job{BoardFacts: store.BoardFacts{Pay: pay(120000, "USD", "year")}}

	check := findCheck(t, jobfit.Judge(job, nil, withTakeHome, jobfit.ExchangeRates{}), "Pay")
	if check.Verdict != jobfit.VerdictUnclear {
		t.Fatalf("with no rates fetched: %+v", check)
	}
}

func TestTheRole(t *testing.T) {
	for _, test := range []struct {
		title  string
		want   jobfit.Verdict
		reason string
	}{
		{"Senior Sales Engineer", jobfit.VerdictNo, `"Sales" in the title`},
		{"Senior Business Analyst - Payments", jobfit.VerdictNo, `"Analyst" in the title`},
		{"Technical Product Manager", jobfit.VerdictNo, `"Manager" in the title`},
		{"Desenvolvedor FrontEnd React - Sênior", jobfit.VerdictYes, "Senior Front-End Engineer"},
		{"Senior Full Stack Developer", jobfit.VerdictYes, "Senior Full-Stack Engineer"},
		{"Product Engineer", jobfit.VerdictYes, "Product Engineer"},
		{"Staff React Native Engineer", jobfit.VerdictYes, "a React role"},
		{"Senior Software Engineer, Agents", jobfit.VerdictUnclear, "an engineering title that names none of your roles"},
		{"Desenvolvedor(a) Backend Pleno", jobfit.VerdictUnclear, "an engineering title that names none of your roles"},
		{"Content Writer, Investment Research", jobfit.VerdictNo, "the title names none of your roles"},
	} {
		check := findCheck(t, jobfit.Judge(store.Job{Title: test.title}, nil, criteria, rates), "Role")
		if check.Verdict != test.want || check.Reason != test.reason {
			t.Errorf("%q: %+v, want %s %q", test.title, check, test.want, test.reason)
		}
	}
}

func TestTheTimezone(t *testing.T) {
	for _, test := range []struct {
		requirement string
		want        jobfit.Verdict
	}{
		{"8am - 5pm EDT", jobfit.VerdictYes},
		{"10am-3pm in your local time zone with flexibility", jobfit.VerdictYes},
		{"At least four hours of weekday overlap with Bangkok (GMT+7)", jobfit.VerdictNo},
		{"EU timezones", jobfit.VerdictUnclear},
	} {
		check := findCheck(t, jobfit.Judge(store.Job{}, facts(t, map[string]any{"timezone_requirement": test.requirement}), criteria, rates), "Timezone")
		if check.Verdict != test.want {
			t.Errorf("%q: %+v, want %s", test.requirement, check, test.want)
		}
	}
	for _, unstated := range []string{"not stated", "Not stated (schedule can remain flexible)", ""} {
		for _, check := range jobfit.Judge(store.Job{}, facts(t, map[string]any{"timezone_requirement": unstated}), criteria, rates).Checks {
			if check.Name == "Timezone" {
				t.Errorf("%q got a timezone check: %+v", unstated, check)
			}
		}
	}
}

func TestTheFitLevel(t *testing.T) {
	good := jobfit.Judge(store.Job{Title: "Senior Frontend Engineer", Location: "LATAM"}, facts(t, map[string]any{"technologies": []string{"TypeScript"}}), criteria, rates)
	unclear := jobfit.Judge(store.Job{Title: "Senior Frontend Engineer", Location: "Remote"}, facts(t, map[string]any{"technologies": []string{"TypeScript"}}), criteria, rates)
	poor := jobfit.Judge(store.Job{Title: "Senior Frontend Engineer", Location: "LATAM"}, facts(t, map[string]any{"technologies": []string{"Java"}}), criteria, rates)

	if good.Level != jobfit.LevelGood || unclear.Level != jobfit.LevelUnclear || poor.Level != jobfit.LevelPoor {
		t.Fatalf("levels = %s, %s, %s", good.Level, unclear.Level, poor.Level)
	}
}

func TestFactsReadWithEvidenceAreJudgedLikeFlatOnes(t *testing.T) {
	job := store.Job{Title: "Senior Front-End Engineer", Location: "Remote"}
	flat := facts(t, map[string]any{"location_restriction": "Anywhere in the Americas", "seniority": "Senior", "technologies": []string{"React"}, "timezone_requirement": "overlap with EST"})
	withEvidence := facts(t, map[string]any{
		"location":             map[string]any{"evidence": "Americas", "restriction": "Anywhere in the Americas", "open_to_brazil": "unclear", "reason": "x"},
		"seniority":            map[string]any{"evidence": "Senior", "as_written": "Senior", "levels": []string{"senior"}},
		"technologies":         map[string]any{"evidence": "React", "value": []string{"React"}},
		"timezone_requirement": map[string]any{"evidence": "EST", "value": "overlap with EST"},
	})
	if left, right := jobfit.Judge(job, flat, criteria, rates), jobfit.Judge(job, withEvidence, criteria, rates); fmt.Sprint(left) != fmt.Sprint(right) {
		t.Fatalf("flat:          %+v\nwith evidence: %+v", left, right)
	}

	for answer, want := range map[string]jobfit.Verdict{"yes": jobfit.VerdictYes, "no": jobfit.VerdictNo} {
		read := facts(t, map[string]any{"location": map[string]any{"evidence": "", "restriction": "Canada", "open_to_brazil": answer, "reason": "x"}})
		if check := findCheck(t, jobfit.Judge(job, read, criteria, rates), "Where they hire"); check.Verdict != want {
			t.Errorf("open_to_brazil %q: %+v, want %s", answer, check, want)
		}
	}
}
