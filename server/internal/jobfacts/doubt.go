package jobfacts

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// The doubts, as the job details show them after "Read again by …: ".
const (
	DoubtOpenToBrazilUnclear = "open to Brazil was unclear"
	DoubtOpenWithoutLocation = "open to Brazil, though no location is stated"
	DoubtWorldwideButLimited = "a worldwide location with a country-limited rule"
	DoubtLevelWithoutMention = "a level read where the posting states none"
)

const notStated = "not stated"

// A worldwide restriction with a country-limited rule in it, as "Worldwide,
// US residents only", by the escalation simulation's own patterns.
var (
	worldwidePattern    = regexp.MustCompile(`(?i)\b(worldwide|global|anywhere)\b`)
	countryLimitPattern = regexp.MustCompile(`(?i)(residen|authori[sz]|eligib|must (live|be (based|located))|\bonly\b)`)
)

// FindDoubt says why a reading of the job's facts is doubtful, or "" when it
// isn't. Doubtful readings are where the job facts benchmark of 2026-09-30
// found the local model missing most and a stronger model fixing most: open
// to Brazil is unclear, or "yes" with no location stated, a worldwide
// location restriction with a country-limited rule in it, and seniority
// levels read where the posting's level is written as "not stated". These
// are the rules of that benchmark's escalation simulation, and only them, so
// its measure holds: about 13% of test postings read again. Facts are read
// by their flattened keys (see store.FlattenJobFacts), in either schema of
// the job_facts prompt.
func FindDoubt(facts json.RawMessage) string {
	values, _, err := store.FlattenJobFacts(facts)
	if err != nil {
		return ""
	}
	restrictionKey := "location"
	if _, has := values[restrictionKey]; !has {
		restrictionKey = "location_restriction"
	}
	restriction := readText(values[restrictionKey])
	switch strings.ToLower(readText(values["location.open_to_brazil"])) {
	case "unclear":
		return DoubtOpenToBrazilUnclear
	case "yes":
		if restriction == "" || strings.EqualFold(restriction, notStated) {
			return DoubtOpenWithoutLocation
		}
	}
	if worldwidePattern.MatchString(restriction) && countryLimitPattern.MatchString(restriction) {
		return DoubtWorldwideButLimited
	}
	if len(readTexts(values["seniority.levels"])) > 0 && strings.EqualFold(readText(values["seniority"]), notStated) {
		return DoubtLevelWithoutMention
	}
	return ""
}

// readText is a fact's value as text: text as it is, a list of text joined,
// anything else empty.
func readText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	return strings.Join(readTexts(raw), ", ")
}

// readTexts is a fact's value as a list of text, without the blank ones.
func readTexts(raw json.RawMessage) []string {
	var texts []string
	if json.Unmarshal(raw, &texts) != nil {
		return nil
	}
	var kept []string
	for _, text := range texts {
		if text = strings.TrimSpace(text); text != "" {
			kept = append(kept, text)
		}
	}
	return kept
}
