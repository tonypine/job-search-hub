package api

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// screenOutAnswer is one reason a posting could screen the owner out at
// once, answered from the fit checks and the facts' evidence. An empty
// verdict is information the fit doesn't judge, such as the contract.
type screenOutAnswer struct {
	Name     string         `json:"name"`
	Verdict  jobfit.Verdict `json:"verdict,omitempty"`
	Answer   string         `json:"answer"`
	Evidence string         `json:"evidence,omitempty"`
}

// buildScreenOutAnswers answers whether they hire from Brazil, the level and
// the timezone from the fit, the years of experience against the owner's,
// and the contract and languages from the facts, each with the posting's
// words the facts rest on. experienceMonths is the owner's, 0 when unknown.
func buildScreenOutAnswers(details store.JobDetails, fit jobfit.Fit, experienceMonths int) []screenOutAnswer {
	facts := map[string]store.JobFactEntry{}
	if details.Facts != nil {
		for _, entry := range details.Facts.Entries {
			facts[entry.Key] = entry
		}
	}
	answerFromCheck := func(checkName, answerName string, factKeys ...string) screenOutAnswer {
		answer := screenOutAnswer{Name: answerName, Verdict: jobfit.VerdictUnclear, Answer: "the posting doesn't say"}
		for _, check := range fit.Checks {
			if check.Name == checkName {
				answer.Verdict, answer.Answer = check.Verdict, check.Reason
			}
		}
		answer.Evidence = getFirstEvidence(facts, factKeys...)
		return answer
	}
	return []screenOutAnswer{
		answerFromCheck("Where they hire", "Hires from Brazil", "location", "location_restriction"),
		answerFromCheck("Level", "Level", "seniority"),
		answerFromCheck("Timezone", "Timezone", "timezone_requirement"),
		answerExperience(facts, experienceMonths),
		{Name: "Contract", Answer: describeContract(facts), Evidence: getFirstEvidence(facts, "contract", "contract_type")},
		{Name: "Languages", Answer: describeLanguages(facts), Evidence: getFirstEvidence(facts, "languages")},
	}
}

// answerExperience compares the years the posting asks for with the owner's:
// met is yes, a year short is unclear, further short is no.
func answerExperience(facts map[string]store.JobFactEntry, experienceMonths int) screenOutAnswer {
	answer := screenOutAnswer{Name: "Experience", Verdict: jobfit.VerdictUnclear, Evidence: getFirstEvidence(facts, "years_of_experience")}
	asked := readYears(facts["years_of_experience"].Value)
	switch {
	case asked == 0:
		answer.Answer = "the posting doesn't say"
	case experienceMonths == 0:
		answer.Answer = fmt.Sprintf("asks for %s; your knowledge base has no confirmed roles with dates", formatYears(asked))
	default:
		answer.Answer = fmt.Sprintf("asks for %s; you have %s", formatYears(asked), formatYears(experienceMonths/12))
		switch {
		case experienceMonths >= asked*12:
			answer.Verdict = jobfit.VerdictYes
		case experienceMonths < (asked-1)*12:
			answer.Verdict = jobfit.VerdictNo
		}
	}
	return answer
}

// readYears is the years a fact asks for, as a number or as text such as
// "5+ years"; 0 when it isn't stated.
func readYears(value json.RawMessage) int {
	var number float64
	if json.Unmarshal(value, &number) == nil {
		return int(number)
	}
	var text string
	if json.Unmarshal(value, &text) != nil {
		return 0
	}
	digits := strings.TrimSpace(text)
	if end := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); end >= 0 {
		digits = digits[:end]
	}
	years, _ := strconv.Atoi(digits)
	return years
}

func formatYears(years int) string {
	if years == 1 {
		return "1 year"
	}
	return fmt.Sprintf("%d years", years)
}

// describeLanguages is the languages the posting asks for.
func describeLanguages(facts map[string]store.JobFactEntry) string {
	var languages []string
	if json.Unmarshal(facts["languages"].Value, &languages) != nil || len(languages) == 0 {
		return "the posting doesn't say"
	}
	return strings.Join(languages, ", ")
}

// countExperienceMonths is how many months the owner's roles span up to now,
// counting months two roles share once. A role without a start is left out;
// one without an end runs to now. A year alone starts in January and ends in
// December.
func countExperienceMonths(roles []store.ProfileEntry, now time.Time) int {
	type span struct{ start, end int }
	current := now.Year()*12 + int(now.Month()) - 1
	var spans []span
	for _, role := range roles {
		start, ok := parseMonth(role.StartMonth, 1)
		if !ok {
			continue
		}
		end := current
		if role.EndMonth != "" {
			if end, ok = parseMonth(role.EndMonth, 12); !ok {
				continue
			}
		}
		if end = min(end, current); end >= start {
			spans = append(spans, span{start, end})
		}
	}
	slices.SortFunc(spans, func(a, b span) int { return a.start - b.start })
	months, counted := 0, -1
	for _, s := range spans {
		if start := max(s.start, counted+1); s.end >= start {
			months += s.end - start + 1
			counted = s.end
		}
	}
	return months
}

// parseMonth reads YYYY-MM, or YYYY with month standing in, as months since
// year zero.
func parseMonth(text string, month int) (int, bool) {
	date, err := time.Parse("2006-01", text)
	if err != nil {
		if date, err = time.Parse("2006", text); err != nil {
			return 0, false
		}
		date = date.AddDate(0, month-1, 0)
	}
	return date.Year()*12 + int(date.Month()) - 1, true
}

func getFirstEvidence(facts map[string]store.JobFactEntry, keys ...string) string {
	for _, key := range keys {
		if evidence := facts[key].Evidence; evidence != "" {
			return evidence
		}
	}
	return ""
}

// describeContract is the contract as the posting writes it, with the kinds
// it names when the facts carry them: "PJ (contractor)".
func describeContract(facts map[string]store.JobFactEntry) string {
	var asWritten string
	for _, key := range []string{"contract", "contract_type"} {
		if json.Unmarshal(facts[key].Value, &asWritten) == nil && asWritten != "" {
			break
		}
	}
	var kinds []string
	_ = json.Unmarshal(facts["contract.kinds"].Value, &kinds)
	switch {
	case asWritten == "" || strings.EqualFold(asWritten, "not stated"):
		if len(kinds) > 0 {
			return strings.Join(kinds, ", ")
		}
		return "the posting doesn't say"
	case len(kinds) > 0 && !strings.EqualFold(asWritten, strings.Join(kinds, ", ")):
		return asWritten + " (" + strings.Join(kinds, ", ") + ")"
	default:
		return asWritten
	}
}
