package jobfacts

import (
	"encoding/json"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

// The doubts, as the job details show them after "Read again by …: ".
const (
	DoubtOpenToBrazilUnclear = "open to Brazil was unclear"
	DoubtOpenWithoutLocation = "open to Brazil, though no location is stated"
	DoubtWorldwideButLimited = "a worldwide location with a country-limited rule"
	DoubtLevelWithoutMention = "a level read where the posting states none"
)

const notStated = "not stated"

// worldwideTerms say a posting hires anywhere.
var worldwideTerms = []string{
	"worldwide", "world wide", "anywhere", "global", "globally", "any country", "any location",
	"qualquer lugar", "mundo todo", "todo o mundo", "cualquier lugar", "todo el mundo",
}

// countryLimitTerms make a rule that limits a hire to some countries, as
// "US only" or "must be authorized to work in the US" do.
var countryLimitTerms = []string{
	"only", "authorized to work", "authorised to work", "work authorization", "work authorisation", "right to work",
	"eligible to work", "citizen", "citizens", "citizenship", "resident", "residents", "residency", "reside", "residing",
	"based in", "located in", "living in", "live in", "visa", "sponsorship",
	"apenas", "somente", "solo", "solamente", "unicamente",
}

// ruleFactKeyParts mark a fact, by its key, as a rule on who can be hired,
// such as work_authorization or visa_sponsorship.
var ruleFactKeyParts = []string{"authoriz", "authoris", "visa", "citizen", "residen", "eligib", "sponsor"}

// FindDoubt says why a reading of the job's facts is doubtful, or "" when it
// isn't. Doubtful readings are where the job facts benchmark of 2026-09-30
// found the local model missing most and a stronger model fixing most: open
// to Brazil is unclear, or "yes" with no location stated, a worldwide
// location with a country-limited rule, and seniority levels the posting
// never states. Facts are read by their flattened keys (see
// store.FlattenJobFacts), in either schema of the job_facts prompt.
func FindDoubt(job store.Job, facts json.RawMessage) string {
	values, evidence, err := store.FlattenJobFacts(facts)
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
		if isNotStated(restriction) {
			return DoubtOpenWithoutLocation
		}
	}

	places := append([]string{job.Location, restriction}, job.OtherLocations...)
	rules := []string{restriction, evidence[restrictionKey]}
	for key, value := range values {
		if isRuleFactKey(key) {
			rules = append(rules, readText(value), evidence[key])
		}
	}
	if hasAnyTerm(places, worldwideTerms) && hasAnyTerm(rules, countryLimitTerms) {
		return DoubtWorldwideButLimited
	}

	if len(readTexts(values["seniority.levels"])) > 0 && isNotStated(readText(values["seniority"])) {
		return DoubtLevelWithoutMention
	}
	return ""
}

func isRuleFactKey(key string) bool {
	if strings.HasPrefix(key, "location") {
		return false
	}
	for _, part := range ruleFactKeyParts {
		if strings.Contains(strings.ToLower(key), part) {
			return true
		}
	}
	return false
}

func isNotStated(text string) bool {
	return text == "" || strings.EqualFold(text, notStated)
}

func hasAnyTerm(texts []string, terms []string) bool {
	for _, text := range texts {
		normalized := wordmatch.Normalize(text)
		for _, term := range terms {
			if wordmatch.Contains(normalized, term) {
				return true
			}
		}
	}
	return false
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
