// Package jobfit judges how well a job fits the owner's criteria, from the
// facts its board publishes and the facts read from its text. The model only
// reads what a posting says; these checks decide what it means.
package jobfit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type Verdict string

const (
	VerdictYes     Verdict = "yes"
	VerdictNo      Verdict = "no"
	VerdictUnclear Verdict = "unclear"
)

type Level string

const (
	LevelGood    Level = "good"
	LevelUnclear Level = "unclear"
	LevelPoor    Level = "poor"
)

// Check is one question about a job, answered with the words that decided it.
type Check struct {
	Name    string  `json:"name"`
	Verdict Verdict `json:"verdict"`
	Reason  string  `json:"reason"`
}

// Fit is poor when any check says no, good when every check says yes, and
// unclear otherwise. An unclear job stays visible; only the owner decides.
type Fit struct {
	Level  Level   `json:"level"`
	Checks []Check `json:"checks"`
}

// readFacts are the facts the checks read, by their keys in the job_facts
// prompt's schema. A renamed fact reads as missing, and its check as unclear.
type readFacts struct {
	LocationRestriction string   `json:"location_restriction"`
	Technologies        []string `json:"technologies"`
	Seniority           string   `json:"seniority"`
	PayInText           string   `json:"pay_in_text"`
}

const notStated = "not stated"

// Judge answers each check for the job. rawFacts may be empty when the job
// has not been read yet.
func Judge(job store.Job, rawFacts json.RawMessage, criteria store.JobCriteria) Fit {
	var facts readFacts
	if len(rawFacts) > 0 {
		_ = json.Unmarshal(rawFacts, &facts)
	}
	checks := []Check{
		checkLocation(job, facts, criteria),
		checkStack(job, facts, criteria),
		checkLevel(job, facts, criteria),
	}
	if pay, applies := checkPay(job, facts, criteria); applies {
		checks = append(checks, pay)
	}
	return Fit{Level: getLevel(checks), Checks: checks}
}

func getLevel(checks []Check) Level {
	allYes := true
	for _, check := range checks {
		if check.Verdict == VerdictNo {
			return LevelPoor
		}
		if check.Verdict != VerdictYes {
			allYes = false
		}
	}
	if allYes {
		return LevelGood
	}
	return LevelUnclear
}

func checkLocation(job store.Job, facts readFacts, criteria store.JobCriteria) Check {
	const name = "Where they hire"
	restriction := facts.LocationRestriction
	if strings.EqualFold(strings.TrimSpace(restriction), notStated) {
		restriction = ""
	}
	texts := append([]string{job.Location, restriction}, job.OtherLocations...)
	if term, found := findTerm(texts, criteria.IneligibleLocationTerms); found {
		return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("says %q", term)}
	}
	if term, found := findTerm(texts, criteria.EligibleLocationTerms); found {
		return Check{Name: name, Verdict: VerdictYes, Reason: fmt.Sprintf("names %q", term)}
	}
	named := strings.TrimSpace(restriction)
	if named == "" && !isOnlyRemote(job.Location) {
		named = job.Location
	}
	if named == "" {
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "the posting doesn't say"}
	}
	return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("names only %q", named)}
}

// isOnlyRemote reports whether a location says nothing about where, such as
// "Remote".
func isOnlyRemote(location string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(location))
	return trimmed == "" || trimmed == "remote"
}

func checkStack(job store.Job, facts readFacts, criteria store.JobCriteria) Check {
	const name = "Stack"
	if len(criteria.Technologies) == 0 {
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "the criteria name no technologies"}
	}
	var matched []string
	for _, wanted := range criteria.Technologies {
		if matchesAny(wanted, facts.Technologies) || containsTechnology(job.Title, wanted) {
			matched = append(matched, wanted)
		}
	}
	switch {
	case len(matched) > 0:
		return Check{Name: name, Verdict: VerdictYes, Reason: strings.Join(matched, ", ")}
	case len(facts.Technologies) > 0:
		return Check{Name: name, Verdict: VerdictNo, Reason: "asks for " + strings.Join(firstOf(facts.Technologies, 4), ", ")}
	default:
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "no technologies read yet"}
	}
}

func checkLevel(job store.Job, facts readFacts, criteria store.JobCriteria) Check {
	const name = "Level"
	if len(criteria.SeniorityLevels) == 0 {
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "the criteria name no levels"}
	}
	seniority := strings.TrimSpace(facts.Seniority)
	if strings.EqualFold(seniority, notStated) {
		seniority = ""
	}
	if term, found := findTerm([]string{job.Title, seniority}, criteria.SeniorityLevels); found {
		return Check{Name: name, Verdict: VerdictYes, Reason: term}
	}
	if seniority != "" {
		return Check{Name: name, Verdict: VerdictNo, Reason: seniority}
	}
	return Check{Name: name, Verdict: VerdictUnclear, Reason: "the posting doesn't say"}
}

// checkPay applies when hourly work is refused and the job pays by the hour,
// or when the criteria set a pay floor.
func checkPay(job store.Job, facts readFacts, criteria store.JobCriteria) (Check, bool) {
	const name = "Pay"
	if criteria.RefuseHourlyWork && isPaidHourly(job, facts) {
		return Check{Name: name, Verdict: VerdictNo, Reason: "paid by the hour"}, true
	}
	floor := criteria.MinimumYearlyPay
	if floor == nil {
		return Check{}, false
	}
	if job.Pay == nil || len(job.Pay.Ranges) == 0 {
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "no pay published"}, true
	}
	payRange := job.Pay.Ranges[0]
	yearlyMaximum, isYearly := getYearlyAmount(payRange.Max, payRange.Interval)
	switch {
	case !strings.EqualFold(payRange.Currency, floor.Currency):
		return Check{Name: name, Verdict: VerdictUnclear, Reason: fmt.Sprintf("paid in %s; the floor is in %s", payRange.Currency, floor.Currency)}, true
	case !isYearly:
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "the pay period isn't stated"}, true
	case yearlyMaximum >= floor.Amount:
		return Check{Name: name, Verdict: VerdictYes, Reason: fmt.Sprintf("up to %.0f %s a year", yearlyMaximum, payRange.Currency)}, true
	default:
		return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("up to %.0f %s a year", yearlyMaximum, payRange.Currency)}, true
	}
}

func isPaidHourly(job store.Job, facts readFacts) bool {
	if job.Pay != nil {
		for _, payRange := range job.Pay.Ranges {
			if payRange.Interval == "hour" {
				return true
			}
		}
	}
	text := strings.ToLower(facts.PayInText)
	return strings.Contains(text, "per hour") || strings.Contains(text, "/hour") || strings.Contains(text, "/hr") || strings.Contains(text, "hourly")
}

var periodsPerYear = map[string]float64{"year": 1, "month": 12, "week": 52}

// getYearlyAmount converts a yearly, monthly or weekly amount to a yearly one;
// isYearly is false for other or unstated periods.
func getYearlyAmount(amount float64, interval string) (float64, bool) {
	periods, known := periodsPerYear[interval]
	return amount * periods, known
}

// findTerm returns the first term found in any text as whole words, ignoring
// case and accents: "US only" is not found in "focus only", and "Senior" is
// found in the Portuguese "Sênior".
func findTerm(texts, terms []string) (string, bool) {
	for _, term := range terms {
		normalizedTerm := normalizeWords(term)
		if normalizedTerm == "" {
			continue
		}
		for _, text := range texts {
			if containsWords(normalizeWords(text), normalizedTerm) {
				return term, true
			}
		}
	}
	return "", false
}

var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e", "í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o", "ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
)

func normalizeWords(text string) string {
	return accents.Replace(strings.ToLower(strings.TrimSpace(text)))
}

func containsWords(text, words string) bool {
	for start := 0; start <= len(text)-len(words); {
		index := strings.Index(text[start:], words)
		if index < 0 {
			return false
		}
		begin, end := start+index, start+index+len(words)
		if (begin == 0 || !isWordCharacter(text[begin-1])) && (end == len(text) || !isWordCharacter(text[end])) {
			return true
		}
		start = begin + 1
	}
	return false
}

func isWordCharacter(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9')
}

// normalizeTechnology compares names without case or punctuation, so
// "Node.js" matches "nodejs" and "React.js" matches "React".
func normalizeTechnology(name string) string {
	var normalized strings.Builder
	for _, character := range strings.ToLower(name) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			normalized.WriteRune(character)
		}
	}
	return strings.TrimSuffix(normalized.String(), "js")
}

func matchesAny(wanted string, technologies []string) bool {
	target := normalizeTechnology(wanted)
	for _, technology := range technologies {
		if target != "" && normalizeTechnology(technology) == target {
			return true
		}
	}
	return false
}

// containsTechnology finds a technology named as a word of a title, such as
// "React" in "Senior React Engineer".
func containsTechnology(title, technology string) bool {
	target := normalizeTechnology(technology)
	for _, word := range strings.FieldsFunc(title, func(character rune) bool {
		return character == ' ' || character == '/' || character == ',' || character == '(' || character == ')'
	}) {
		if target != "" && normalizeTechnology(word) == target {
			return true
		}
	}
	return false
}

func firstOf(items []string, count int) []string {
	if len(items) <= count {
		return items
	}
	return items[:count]
}
