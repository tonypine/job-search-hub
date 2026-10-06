// Package jobfit judges how well a job fits the owner's criteria, from the
// facts its board publishes and the facts read from its text. The model only
// reads what a posting says; these checks decide what it means.
package jobfit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
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

// readFacts are the facts the checks read, flattened (see
// store.FlattenJobFacts), by their keys in the job_facts prompt's schemas:
// the flat one, then the one with evidence. A renamed fact reads as missing,
// and its check as unclear.
type readFacts struct {
	LocationRestriction string   `json:"location_restriction"`
	Location            string   `json:"location"`
	OpenToBrazil        string   `json:"location.open_to_brazil"`
	Technologies        []string `json:"technologies"`
	Seniority           string   `json:"seniority"`
	SeniorityLevels     []string `json:"seniority.levels"`
	PayInText           string   `json:"pay_in_text"`
	ContractType        string   `json:"contract_type"`
	Contract            string   `json:"contract"`
	TimezoneRequirement string   `json:"timezone_requirement"`
}

const notStated = "not stated"

// Judge answers each check for the job. rawFacts may be empty when the job
// has not been read yet; rates may be empty, which leaves foreign pay unclear.
func Judge(job store.Job, rawFacts json.RawMessage, criteria store.JobCriteria, rates ExchangeRates) Fit {
	var facts readFacts
	if len(rawFacts) > 0 {
		_ = json.Unmarshal(store.FlattenJobFactsToJSON(rawFacts), &facts)
	}
	if facts.LocationRestriction == "" {
		facts.LocationRestriction = facts.Location
	}
	if facts.ContractType == "" {
		facts.ContractType = facts.Contract
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

// CouldFit reports whether a posting on a board the owner doesn't watch is
// worth storing, from its title and locations alone, before any facts are
// read: its title names one of the roles, and where it hires isn't ruled
// out. An engineering title naming none of the roles is left out: on large
// boards those are most postings, and they never get past unclear.
func CouldFit(posting store.JobPosting, criteria store.JobCriteria) bool {
	job := store.Job{Title: posting.Title, Location: posting.Location, BoardFacts: store.BoardFacts{OtherLocations: posting.OtherLocations}}
	return checkRole(job, criteria).Verdict == VerdictYes && checkLocation(job, readFacts{}, criteria).Verdict != VerdictNo
}

// IsRoleRuledOut reports whether a job's title alone rules it out.
func IsRoleRuledOut(job store.Job, criteria store.JobCriteria) bool {
	return checkRole(job, criteria).Verdict == VerdictNo
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

// engineeringSpecialties name engineering work without an engineering noun,
// as "Front-end React Sr" does. A role keyword without one, such as "product"
// or "growth", also needs an engineering noun in the title, so "Head of
// Growth" isn't read as "Growth Engineer".
var engineeringSpecialties = []string{"frontend", "backend", "fullstack"}

func checkRole(job store.Job, criteria store.JobCriteria) Check {
	const name = "Role"
	title := normalizeTitle(job.Title)
	if term, found := findTerm([]string{job.Title}, criteria.ExcludedRoleTerms); found {
		return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("%q in the title", term)}
	}
	isEngineering := hasAnyTerm([]string{job.Title}, engineeringNouns...)
	for _, role := range criteria.Roles {
		keyword := getRoleKeyword(role, criteria.SeniorityLevels)
		if keyword != "" && wordmatch.Contains(title, keyword) && (isEngineering || hasAnyTerm([]string{keyword}, engineeringSpecialties...)) {
			return Check{Name: name, Verdict: VerdictYes, Reason: role}
		}
	}
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
	// The model's own reading of the rule, when the prompt asks for one, is
	// more reliable than matching terms in it.
	switch facts.OpenToBrazil {
	case "yes":
		return Check{Name: name, Verdict: VerdictYes, Reason: "open to someone in Brazil"}
	case "no":
		return Check{Name: name, Verdict: VerdictNo, Reason: "not open to someone in Brazil"}
	}
	restriction := facts.LocationRestriction
	if strings.EqualFold(strings.TrimSpace(restriction), notStated) {
		restriction = ""
	}
	texts := append([]string{job.Location, restriction}, job.OtherLocations...)
	if term, found := findTerm(texts, criteria.IneligibleLocationTerms); found {
		return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("says %q", term)}
	}
	if term, found := findTerm(texts, getEligibleTerms(criteria)); found {
		return Check{Name: name, Verdict: VerdictYes, Reason: fmt.Sprintf("names %q", term)}
	}
	named := strings.TrimSpace(restriction)
	if named == "" && !isOnlyRemote(job.Location) {
		named = job.Location
	}
	if named == "" {
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "the posting doesn't say"}
	}
	if softener := getSoftener(named); softener != "" && !hasResidencyRule(texts) {
		if isOnlyRemote(job.Location) || getSoftener(job.Location) != "" {
			return Check{Name: name, Verdict: VerdictUnclear, Reason: fmt.Sprintf("only %s: %q", softener, named)}
		}
		named = job.Location
	}
	return Check{Name: name, Verdict: VerdictNo, Reason: fmt.Sprintf("names only %q", named)}
}

// placeSpellings are the ways postings write a place, in English,
// Portuguese and Spanish.
var placeSpellings = map[string][]string{
	"Brazil":        {"Brazil", "Brasil"},
	"Latin America": {"Latin America", "LATAM", "América Latina", "Latinoamérica"},
	"South America": {"South America", "América do Sul", "América del Sur", "Sudamérica", "Suramérica"},
	"Americas":      {"Americas", "Américas"},
}

// placeRegions are the regions that include a place.
var placeRegions = map[string][]string{
	"Brazil":        {"Latin America", "South America"},
	"Latin America": {"Americas"},
	"South America": {"Americas"},
}

// getEligibleTerms returns the owner's eligible terms and home country, with
// every spelling of the places they name and of the regions that include
// them: a posting for "Latin America only" or "Brasil" is open to someone
// in Brazil.
func getEligibleTerms(criteria store.JobCriteria) []string {
	terms := append(slices.Clone(criteria.EligibleLocationTerms), criteria.HomeCountry)
	pending := slices.Clone(terms)
	seen := map[string]bool{}
	for len(pending) > 0 {
		place := getPlace(pending[0])
		pending = pending[1:]
		if place == "" || seen[place] {
			continue
		}
		seen[place] = true
		terms = append(terms, placeSpellings[place]...)
		pending = append(pending, placeRegions[place]...)
	}
	return terms
}

// getPlace returns the place a term spells, as "Brazil" for "Brasil", or ""
// for a term that spells none of placeSpellings.
func getPlace(term string) string {
	for place, spellings := range placeSpellings {
		for _, spelling := range spellings {
			if wordmatch.Normalize(spelling) == wordmatch.Normalize(term) {
				return place
			}
		}
	}
	return ""
}

// A place a posting names reads as a residency rule, unless it is only
// preferred, as in "Remote, North America preferred", or only sets working
// hours, as in "Remote (US time zones)": those postings may still hire in
// Brazil. A word about residence keeps it a rule, and so does a word that
// requires, unless it only requires hours, as in "Must overlap 4 hours with
// EST". A location that names a place on its own, as "United States
// (Remote)" does, stays a rule too.
var (
	requirementWords = []string{
		"must", "only", "required", "requires", "require", "requirement", "mandatory", "need to", "needs to",
		"apenas", "somente", "obrigatorio", "obrigatoria", "obrigatoriamente", "requisito",
		"solo", "solamente", "unicamente", "excluyente",
	}
	residenceWords = []string{
		"resident", "residents", "residency", "reside", "residing", "based in", "located in", "living in", "live in",
		"citizen", "citizens", "citizenship", "authorized", "authorization", "eligible", "eligibility", "right to work",
		"residir", "residente", "residentes", "morar",
	}
	preferenceWords = []string{
		"preferred", "preferably", "prefer", "prefers", "preference", "ideally", "nice to have", "a plus", "bonus",
		"preferencialmente", "preferencia", "preferible", "preferiblemente", "desejavel", "deseable", "diferencial",
	}
	workingHoursWords = []string{
		"time zone", "time zones", "timezone", "timezones", "hours", "overlap", "fuso horario", "horario",
	}
	zoneNames = []string{
		"eastern time", "central time", "mountain time", "pacific time",
		"utc", "gmt", "est", "edt", "pst", "pdt", "cst", "cdt", "mst", "mdt", "cet", "cest", "brt",
	}
	softeningWords = slices.Concat(preferenceWords, workingHoursWords, zoneNames)
	// qualifierWords soften the place they come with. A zone name alone
	// doesn't: in "Austin TX CST" it only tags the city.
	qualifierWords = slices.Concat(preferenceWords, workingHoursWords)
	hoursWords     = slices.Concat(workingHoursWords, zoneNames)
	// zoneOnlyWords are the words of a text that names only time zones, as
	// "UTC-5 to UTC+1" and "Remote EST" do. Beside a zone name alone, a
	// short word is a place: "IN EST" names Indiana and "AT CET" Austria.
	zoneOnlyWords = slices.Concat(zoneNames, []string{"to", "or", "and", "remote"})
	// hoursOnlyWords are the words of a text that requires working hours, or
	// a time zone, and names no place, as "EST only" and "Must be able to
	// work EST hours" do. Country codes like "de" and "no" are left out.
	hoursOnlyWords = slices.Concat(hoursWords, requirementWords, []string{
		"remote", "remoto", "remota", "fully", "be", "is", "are", "able", "to", "work", "working", "during", "with", "within", "in",
		"the", "a", "an", "at", "least", "of", "our", "your", "and", "or", "core", "business", "hrs", "am", "pm",
		"ser", "capaz", "trabalhar", "em", "com", "ou", "durante", "pelo", "menos", "con", "trabajar", "poder", "minimo",
	})
)

// isOnlyAboutHours reports whether a text sets working hours or a time zone
// and names no place. Filler words like "in" and "at" count only beside a
// word that requires or an hours word, as in "Must be in EST".
func isOnlyAboutHours(text string) bool {
	if !hasAnyTerm([]string{text}, hoursWords...) {
		return false
	}
	if !hasAnyTerm([]string{text}, requirementWords...) && !hasAnyTerm([]string{text}, workingHoursWords...) {
		return isOnlyWordsOf(text, zoneOnlyWords)
	}
	return isOnlyWordsOf(text, hoursOnlyWords)
}

// hasResidencyRule reports whether any part of the texts requires where
// someone lives. A requirement on working hours alone isn't one.
func hasResidencyRule(texts []string) bool {
	for _, text := range texts {
		for _, part := range getParts(text) {
			if hasAnyTerm([]string{part}, residenceWords...) || hasAnyTerm([]string{part}, requirementWords...) && !isOnlyAboutHours(part) {
				return true
			}
		}
	}
	return false
}

// getSoftener returns "a preference" or "a time zone" when every place a
// posting names comes with one, and "" when any place stands on its own:
// "Remote (US, EST preferred)", "Austin, TX (CST)" and "Hiring in the US
// with PST hours" still name only the US.
func getSoftener(named string) string {
	var places []string
	for _, part := range getParts(named) {
		switch {
		case isOnlyRemote(part):
		case len(places) > 0 && isOnlyWordsOf(part, preferenceWords):
			// A bare "preferably" softens the place before it, as in "US or
			// Canada, preferably".
			places[len(places)-1] += ", " + part
		default:
			places = append(places, part)
		}
	}
	if len(places) == 0 {
		return ""
	}
	for _, place := range places {
		if !isSoftened(place) {
			return ""
		}
	}
	if hasAnyTerm([]string{named}, preferenceWords...) {
		return "a preference"
	}
	return "a time zone"
}

// partJoiners start a part of their own, so what follows them qualifies
// only itself: in "Hiring in the US with PST hours" the hours aren't the
// US's.
var partJoiners = regexp.MustCompile(`(?i)\s+(with|com|con)\s+`)

// getParts splits a text at commas, semicolons, parentheses and
// partJoiners.
func getParts(text string) []string {
	return strings.FieldsFunc(partJoiners.ReplaceAllString(text, ","), isPartSeparator)
}

func isPartSeparator(character rune) bool {
	return strings.ContainsRune(",;()", character)
}

// placeJoiners are read as slashes. A hyphen joins only with spaces around
// it, since "UTC-5" is one time zone.
var placeJoiners = strings.NewReplacer(" - ", "/", " – ", "/", "—", "/", "|", "/")

// isSoftened reports whether a place comes with a preference or working
// hours, or is only a time zone, as "EST" and "EST only" are. A zone
// name beside a place only tags it, as in "Austin TX CST". Places joined by
// slashes, spaced dashes or bars share a qualifier written before the first
// or after the last of them, as in "preferably US/Canada" and "US/Canada
// time zones", unless it stands alone, as in "US/EST" or "US - EST".
func isSoftened(place string) bool {
	alternatives := strings.Split(placeJoiners.Replace(place), "/")
	for _, edge := range []string{alternatives[0], alternatives[len(alternatives)-1]} {
		if hasAnyTerm([]string{edge}, qualifierWords...) && !isOnlyWordsOf(edge, softeningWords) {
			return true
		}
	}
	for _, alternative := range alternatives {
		if !isOnlyRemote(alternative) && !hasAnyTerm([]string{alternative}, qualifierWords...) && !isOnlyAboutHours(alternative) {
			return false
		}
	}
	return true
}

// isOnlyWordsOf reports whether every word of a text, other than numbers,
// belongs to one of the terms, as "preferably" and "EST" do.
func isOnlyWordsOf(text string, terms []string) bool {
	known := map[string]bool{}
	for _, term := range terms {
		for _, word := range getWords(term) {
			known[word] = true
		}
	}
	for _, word := range getWords(text) {
		if !known[word] && !isNumber(word) {
			return false
		}
	}
	return true
}

// remoteWords say a job is remote without saying where, as in "Fully
// remote", "100% remoto" or "Trabalho remoto".
var remoteWords = map[string]bool{"remote": true, "remoto": true, "remota": true, "fully": true, "trabalho": true}

// isOnlyRemote reports whether a location says nothing about where, such as
// "Remote", or the "9:00" of "9:00 - 17:00 EST".
func isOnlyRemote(location string) bool {
	for _, word := range getWords(location) {
		if !remoteWords[word] && !isNumber(word) {
			return false
		}
	}
	return true
}

func isNumber(word string) bool {
	return strings.Trim(word, "0123456789") == ""
}

// getWords returns the normalized words of a text: "100% Remoto" has "100"
// and "remoto".
func getWords(text string) []string {
	return strings.FieldsFunc(wordmatch.Normalize(text), func(character rune) bool {
		return (character < 'a' || character > 'z') && (character < '0' || character > '9')
	})
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
	case job.TextMissingReason != "" && len(strings.TrimSpace(job.Description)) < store.AlertSnippetLength:
		return Check{Name: name, Verdict: VerdictUnclear, Reason: job.TextMissingReason}
	case strings.TrimSpace(job.Description) == "":
		return Check{Name: name, Verdict: VerdictUnclear, Reason: "the listing has no text to read"}
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
	levelTexts := facts.SeniorityLevels
	if len(levelTexts) == 0 {
		levelTexts = []string{expandLevelAbbreviations(job.Title), expandLevelAbbreviations(seniority)}
	}
	for _, wanted := range criteria.SeniorityLevels {
		if _, found := findTerm(levelTexts, []string{expandLevelAbbreviations(wanted)}); found {
			return Check{Name: name, Verdict: VerdictYes, Reason: wanted}
		}
	}
	if seniority != "" || len(facts.SeniorityLevels) > 0 {
		return Check{Name: name, Verdict: VerdictNo, Reason: describeLevel(seniority, facts.SeniorityLevels)}
	}
	if level := juniorOrMidLevelPattern.FindString(wordmatch.Normalize(job.Title)); level != "" {
		return Check{Name: name, Verdict: VerdictNo, Reason: describeLevel(level, nil)}
	}
	return Check{Name: name, Verdict: VerdictUnclear, Reason: "the posting doesn't say"}
}

// juniorOrMidLevelPattern finds a junior or mid level in a title with no
// seniority fact yet. A bare "mid" isn't one: "Mid-Market" is a segment.
var juniorOrMidLevelPattern = regexp.MustCompile(`\b(junior|jr|pleno|ssr|semi[- ]?senior|mid[- ]level|intern|estagiario|trainee)\b`)

// levelAbbreviations turn the levels postings abbreviate, in English, Spanish
// and Portuguese, into the words the criteria use. Semi-senior goes first, so
// its "senior" isn't read as senior.
var levelAbbreviations = []struct {
	pattern *regexp.Regexp
	level   string
}{
	{regexp.MustCompile(`\b(semi[- ]?senior|ssr|pleno)\b`), "mid"},
	{regexp.MustCompile(`\bsr\b`), "senior"},
	{regexp.MustCompile(`\bjr\b`), "junior"},
}

// expandLevelAbbreviations returns the text normalized, with each
// abbreviated level written out: "React Sr." becomes "react senior.".
func expandLevelAbbreviations(text string) string {
	expanded := wordmatch.Normalize(text)
	for _, abbreviation := range levelAbbreviations {
		expanded = abbreviation.pattern.ReplaceAllString(expanded, abbreviation.level)
	}
	return expanded
}

// describeLevel names the level as the posting wrote it, followed by the
// level it means when that reads differently: "Pleno (mid)".
func describeLevel(seniority string, canonicalLevels []string) string {
	meaning := strings.Join(canonicalLevels, ", ")
	if len(canonicalLevels) == 0 {
		meaning = expandLevelAbbreviations(seniority)
	}
	switch {
	case seniority == "":
		return meaning
	case meaning == wordmatch.Normalize(seniority):
		return seniority
	default:
		return seniority + " (" + meaning + ")"
	}
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

// RateSource gives the exchange rates from one currency to the others.
type RateSource interface {
	GetRates(ctx context.Context, base string) (map[string]float64, error)
}

// ReadInputs reads what a fit is judged against: the saved criteria, and the
// exchange rates into the take-home currency when the criteria set one.
// Missing rates leave foreign pay unclear rather than failing.
func ReadInputs(ctx context.Context, hub *store.Store, rateSource RateSource) (store.JobCriteria, ExchangeRates, error) {
	saved, err := hub.GetJobCriteria(ctx)
	if err != nil {
		return store.JobCriteria{}, ExchangeRates{}, err
	}
	takeHome := saved.Criteria.TakeHome
	if takeHome == nil {
		return saved.Criteria, ExchangeRates{}, nil
	}
	rates := ExchangeRates{Base: takeHome.Currency}
	if rates.PerBase, err = rateSource.GetRates(ctx, takeHome.Currency); err != nil {
		slog.Warn("exchange rates unavailable; foreign pay reads unclear", "error", err)
	}
	return saved.Criteria, rates, nil
}
