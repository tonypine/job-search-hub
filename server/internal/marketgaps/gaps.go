// Package marketgaps finds the technologies good-fit postings keep asking
// for that the owner's knowledge base never names, and has a routed model
// plan how to close each one.
package marketgaps

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

const (
	// minimumJobCount is how many good fits must ask for a technology before
	// it counts as recurring.
	minimumJobCount = 3
	// maximumGaps keeps the list to what can be worked on.
	maximumGaps = 15
	// maximumNameWords bounds the multi-word names matched against the
	// knowledge base, such as "React Native" or "Amazon Web Services".
	maximumNameWords = 3
)

// GoodFit is a good-fit job and the technologies read from its posting.
type GoodFit struct {
	JobID        uuid.UUID
	Technologies []string
}

// Gap is a recurring technology the knowledge base lacks: its most common
// spelling, and the good fits that ask for it.
type Gap struct {
	Technology string
	JobIDs     []uuid.UUID
}

// FindGaps counts each technology once per good fit, by its canonical key,
// and returns those asked for by at least minimumJobCount good fits that the
// knowledge base text never names, the most asked for first.
func FindGaps(goodFits []GoodFit, knowledgeBase string) []Gap {
	known := collectKnownKeys(knowledgeBase)
	jobsByKey := map[string][]uuid.UUID{}
	spellings := map[string]map[string]int{}
	for _, fit := range goodFits {
		seen := map[string]bool{}
		for _, technology := range fit.Technologies {
			key := canonicalize(technology)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			jobsByKey[key] = append(jobsByKey[key], fit.JobID)
			if spellings[key] == nil {
				spellings[key] = map[string]int{}
			}
			spellings[key][strings.TrimSpace(technology)]++
		}
	}
	var gaps []Gap
	for key, jobIDs := range jobsByKey {
		if len(jobIDs) < minimumJobCount || known[key] {
			continue
		}
		gaps = append(gaps, Gap{Technology: pickCommonSpelling(spellings[key]), JobIDs: jobIDs})
	}
	slices.SortFunc(gaps, func(left, right Gap) int {
		if byCount := cmp.Compare(len(right.JobIDs), len(left.JobIDs)); byCount != 0 {
			return byCount
		}
		return cmp.Compare(left.Technology, right.Technology)
	})
	return gaps[:min(len(gaps), maximumGaps)]
}

// canonicalize keys a technology by its letters and digits in lowercase, so
// "Node.JS", "nodejs" and "Node JS" are one technology.
func canonicalize(name string) string {
	var key strings.Builder
	for _, character := range strings.ToLower(name) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			key.WriteRune(character)
		}
	}
	return key.String()
}

// collectKnownKeys keys every run of up to maximumNameWords words in the
// text, so a technology named anywhere in it, in one word or several, is
// known.
func collectKnownKeys(text string) map[string]bool {
	words := strings.FieldsFunc(text, func(character rune) bool {
		return unicode.IsSpace(character) || strings.ContainsRune(",;:()[]{}\"'/|", character)
	})
	known := map[string]bool{}
	for start := range words {
		for length := 1; length <= maximumNameWords && start+length <= len(words); length++ {
			if key := canonicalize(strings.Join(words[start:start+length], " ")); key != "" {
				known[key] = true
			}
		}
	}
	return known
}

// pickCommonSpelling returns the spelling postings use most, the first in
// alphabetical order when tied.
func pickCommonSpelling(counts map[string]int) string {
	var best string
	for spelling, count := range counts {
		if best == "" || count > counts[best] || (count == counts[best] && spelling < best) {
			best = spelling
		}
	}
	return best
}
