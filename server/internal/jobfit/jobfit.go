// Package jobfit judges how well a job fits the owner's criteria, from the
// facts its board publishes and the facts read from its text. The model only
// reads what a posting says; these checks decide what it means.
package jobfit

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
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
	ContractType        string   `json:"contract_type"`
	TimezoneRequirement string   `json:"timezone_requirement"`
}

const notStated = "not stated"

// Judge answers each check for the job. rawFacts may be empty when the job
// has not been read yet; rates may be empty, which leaves foreign pay unclear.
func Judge(job store.Job, rawFacts json.RawMessage, criteria store.JobCriteria, rates ExchangeRates) Fit {
	var facts readFacts
	if len(rawFacts) > 0 {
		_ = json.Unmarshal(rawFacts, &facts)
	}
	checks := []Check{
		checkRole(job, criteria),
		checkLocation(job, facts, criteria),
		checkStack(job, facts, criteria),
		checkLevel(job, facts, criteria),
	}
	if timezone, applies := checkTimezone(facts, criteria); applies {
		checks = append(checks, timezone)
	}
	if pay, applies := checkPay(job, facts, criteria, rates); applies {
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

// genericRoleWords are left out of a criteria role when matching titles,
// with the criteria's own levels: "Senior Front-End Engineer" matches any
// title with "frontend" in it.
var genericRoleWords = map[string]bool{"engineer": true, "developer": true, "software": true, "sr": true, "jr": true}

// engineeringNouns mark a title as engineering work, in English and Portuguese.
var engineeringNouns = []string{
	"engineer", "developer", "desenvolvedor", "desenvolvedora", "programmer", "programador", "dev", "architect", "arquiteto", "swe",
}

func checkRole(job store.Job, criteria store.JobCriteria) Check {
	const name = "Role"
	title := normalizeTitle(job.Title)
	if term, found := findTerm([]string{job.Title}, criteria.ExcludedRoleTerms); found {
		return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("%q in the title", term)}
	}
	for _, role := range criteria.Roles {
		if keyword := getRoleKeyword(role, criteria.SeniorityLevels); keyword != "" && wordmatch.Contains(title, keyword) {
			return Check{Name: name, Verdict: VerdictYes, Reason: role}
		}
	}
	isEngineering := hasAnyTerm([]string{job.Title}, engineeringNouns...)
	for _, technology := range criteria.Technologies {
		if isEngineering && containsTechnology(job.Title, technology) {
			return Check{Name: name, Verdict: VerdictYes, Reason: "a " + technology + " role"}
		}
	}
	if isEngineering {
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "an engineering title that names none of your roles"}
	}
	return Check{Name: name, Verdict: VerdictNo, Reason: "the title names none of your roles"}
}

// getRoleKeyword is what a criteria role must share with a title: its words
// without the level and generic words, e.g. "frontend" for "Senior Front-End
// Engineer" and "ai product" for "AI Product Engineer".
func getRoleKeyword(role string, levels []string) string {
	excluded := map[string]bool{}
	for _, level := range levels {
		excluded[wordmatch.Normalize(level)] = true
	}
	var kept []string
	for _, word := range strings.Fields(normalizeTitle(role)) {
		if !genericRoleWords[word] && !excluded[word] {
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
}

// titleSpellings join the spellings of the same kind of role.
var titleSpellings = strings.NewReplacer("front-end", "frontend", "front end", "frontend", "full-stack", "fullstack", "full stack", "fullstack",
	"back-end", "backend", "back end", "backend")

func normalizeTitle(title string) string {
	return titleSpellings.Replace(wordmatch.Normalize(title))
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

// checkTimezone applies when the posting states the hours it requires; most
// don't.
func checkTimezone(facts readFacts, criteria store.JobCriteria) (Check, bool) {
	const name = "Timezone"
	requirement := strings.TrimSpace(facts.TimezoneRequirement)
	if requirement == "" || strings.HasPrefix(strings.ToLower(requirement), notStated) {
		return Check{}, false
	}
	texts := []string{requirement}
	if term, found := findTerm(texts, criteria.UnworkableTimezoneTerms); found {
		return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("asks for %q", term)}, true
	}
	if term, found := findTerm(texts, criteria.WorkableTimezoneTerms); found {
		return Check{Name: name, Verdict: VerdictYes, Reason: fmt.Sprintf("asks for %q", term)}, true
	}
	return Check{Name: name, Verdict: VerdictUnclear, Reason: fmt.Sprintf("asks for %q", shorten(requirement, 60))}, true
}

// shorten cuts text to at most length characters, marking the cut.
func shorten(text string, length int) string {
	runes := []rune(text)
	if len(runes) <= length {
		return text
	}
	return strings.TrimSpace(string(runes[:length])) + "…"
}

// ExchangeRates convert pay to the take-home currency: PerBase[c] is how much
// of currency c one unit of Base buys.
type ExchangeRates struct {
	Base    string
	PerBase map[string]float64
}

func (rates ExchangeRates) convertToBase(amount float64, currency string) (float64, bool) {
	if strings.EqualFold(currency, rates.Base) {
		return amount, true
	}
	rate, known := rates.PerBase[strings.ToUpper(currency)]
	if !known || rate <= 0 {
		return 0, false
	}
	return amount / rate, true
}

// hiring is one way the owner could be hired for a job.
type hiring struct {
	name     string
	takeHome store.HiringTakeHome
}

// checkPay applies when hourly work is refused and the job pays by the hour,
// or when the criteria set a take-home and the posting publishes pay: most
// postings publish none, and a missing salary says nothing against a job. It estimates what the top of the
// published range would leave each month under every way the owner could be
// hired and every period the pay could be for; the answer is no only when
// even the best case is under the minimum.
func checkPay(job store.Job, facts readFacts, criteria store.JobCriteria, rates ExchangeRates) (Check, bool) {
	const name = "Pay"
	if criteria.RefuseHourlyWork && isPaidHourly(job, facts) {
		return Check{Name: name, Verdict: VerdictNo, Reason: "paid by the hour"}, true
	}
	takeHome := criteria.TakeHome
	if takeHome == nil {
		return Check{}, false
	}
	if job.Pay == nil || len(job.Pay.Ranges) == 0 {
		return Check{}, false
	}
	payRange := job.Pay.Ranges[0]
	monthlyAmounts := getPossibleMonthlyAmounts(payRange.Max, payRange.Interval)
	if len(monthlyAmounts) == 0 {
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "paid by the " + payRange.Interval}, true
	}

	var estimates []float64
	for _, monthly := range monthlyAmounts {
		converted, known := rates.convertToBase(monthly, payRange.Currency)
		if !known {
			return Check{Name: name, Verdict: VerdictUnclear, Reason: fmt.Sprintf("no exchange rate from %s to %s", payRange.Currency, takeHome.Currency)}, true
		}
		for _, possible := range getPossibleHirings(job, facts, payRange.Currency, *takeHome) {
			estimates = append(estimates, converted*possible.takeHome.Share*possible.takeHome.PaymentsPerYear/12)
		}
	}
	lowest, highest := slices.Min(estimates), slices.Max(estimates)
	isEstimateExact := formatAmount(lowest, takeHome.Currency) == formatAmount(highest, takeHome.Currency)
	switch {
	case highest < takeHome.MinimumMonthly:
		qualifier := "at most"
		if isEstimateExact {
			qualifier = "about"
		}
		return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("%s %s a month take-home, under the %s minimum",
			qualifier, formatAmount(highest, takeHome.Currency), formatAmount(takeHome.MinimumMonthly, takeHome.Currency))}, true
	case lowest >= takeHome.MinimumMonthly:
		qualifier := "at least"
		if isEstimateExact {
			qualifier = "about"
		}
		reason := fmt.Sprintf("%s %s a month take-home", qualifier, formatAmount(lowest, takeHome.Currency))
		if takeHome.TargetMonthly > 0 {
			reason += fmt.Sprintf(", %.0f%% of the target", lowest/takeHome.TargetMonthly*100)
		}
		return Check{Name: name, Verdict: VerdictYes, Reason: reason}, true
	default:
		return Check{Name: name, Verdict: VerdictUnclear, Reason: fmt.Sprintf("%s to %.1fk a month take-home, depending on the contract or pay period",
			formatAmount(lowest, takeHome.Currency), highest/1000)}, true
	}
}

// getPossibleMonthlyAmounts reads an amount as monthly pay. A board that
// doesn't state the period could mean a year or a month, so both are
// returned; hourly and daily pay are not converted.
func getPossibleMonthlyAmounts(amount float64, interval string) []float64 {
	switch interval {
	case "year":
		return []float64{amount / 12}
	case "month":
		return []float64{amount}
	case "week":
		return []float64{amount * 52 / 12}
	case "":
		return []float64{amount / 12, amount}
	default:
		return nil
	}
}

// getPossibleHirings reads how the owner would be hired from the Contract
// fact and the employment type: CLT or an employer of record means CLT, PJ
// means PJ, and a contractor is PJ at home or a foreign contractor abroad. An
// unstated contract could be either kind.
func getPossibleHirings(job store.Job, facts readFacts, currency string, takeHome store.TakeHome) []hiring {
	clt := hiring{"CLT", takeHome.CLT}
	pj := hiring{"PJ", takeHome.PJ}
	foreignContractor := hiring{"contractor", takeHome.ForeignContractor}
	contract := []string{facts.ContractType, job.EmploymentType}
	isLocal := strings.EqualFold(currency, takeHome.Currency)

	switch {
	case hasAnyTerm(contract, "clt", "eor", "employer of record"):
		return []hiring{clt}
	case hasAnyTerm(contract, "pj"):
		return []hiring{pj}
	case hasAnyTerm(contract, "contractor", "contract", "freelance", "freelancer", "independent"):
		if isLocal {
			return []hiring{pj}
		}
		return []hiring{foreignContractor}
	case hasAnyTerm(contract, "employee", "employment", "permanent"):
		return []hiring{clt}
	case isLocal:
		return []hiring{clt, pj}
	default:
		return []hiring{foreignContractor, clt}
	}
}

func hasAnyTerm(texts []string, terms ...string) bool {
	_, found := findTerm(texts, terms)
	return found
}

// formatAmount writes an amount in thousands, such as "BRL 20.3k".
func formatAmount(amount float64, currency string) string {
	return fmt.Sprintf("%s %.1fk", currency, amount/1000)
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

// findTerm returns the first term found in any text as whole words, ignoring
// case and accents: "US only" is not found in "focus only", and "Senior" is
// found in the Portuguese "Sênior".
func findTerm(texts, terms []string) (string, bool) {
	for _, term := range terms {
		normalizedTerm := wordmatch.Normalize(term)
		if normalizedTerm == "" {
			continue
		}
		for _, text := range texts {
			if wordmatch.Contains(wordmatch.Normalize(text), normalizedTerm) {
				return term, true
			}
		}
	}
	return "", false
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
