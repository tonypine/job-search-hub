// Package voice picks the messages that show how the owner writes, so drafts
// can sound like them: recent ones, in each language they write in.
package voice

import (
	"fmt"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

// Languages the owner writes in.
const (
	English    = "English"
	Portuguese = "Portuguese"
)

// SamplesPerLanguage is how many messages a draft reads per language.
const SamplesPerLanguage = 6

// portugueseWords and englishWords are common words that tell the two
// languages apart in a short message.
var (
	portugueseWords = []string{"voce", "nao", "obrigado", "obrigada", "que", "para", "com", "uma", "mas", "tudo", "bem", "estou", "sim", "aqui", "muito", "abraco", "abs"}
	englishWords    = []string{"you", "the", "and", "thanks", "thank", "for", "with", "this", "that", "would", "i'm", "have", "are", "best", "regards"}
)

// Sample is one message chosen to show how the owner writes.
type Sample struct {
	Language string
	store.VoiceSample
}

// PickSamples keeps, from messages the most recent first, up to
// SamplesPerLanguage per language, skipping one that repeats an earlier one.
func PickSamples(messages []store.VoiceSample) []Sample {
	var picked []Sample
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, message := range messages {
		language := DetectLanguage(message.Content)
		key := wordmatch.Normalize(message.Content)
		if language == "" || counts[language] >= SamplesPerLanguage || seen[key] {
			continue
		}
		seen[key] = true
		counts[language]++
		picked = append(picked, Sample{Language: language, VoiceSample: message})
	}
	return picked
}

// DetectLanguage tells English from Portuguese by their common words, and
// returns "" when a message has too few of either.
func DetectLanguage(text string) string {
	normalized := wordmatch.Normalize(text)
	count := func(words []string) int {
		found := 0
		for _, word := range words {
			if wordmatch.Contains(normalized, word) {
				found++
			}
		}
		return found
	}
	portuguese, english := count(portugueseWords), count(englishWords)
	switch {
	case portuguese >= 2 && portuguese > english:
		return Portuguese
	case english >= 2 && english > portuguese:
		return English
	default:
		return ""
	}
}

// FormatSamples writes the samples for a prompt, each with its language and
// month; how to use them is the prompt's to say.
func FormatSamples(samples []Sample) string {
	if len(samples) == 0 {
		return "No messages of mine are in the hub yet."
	}
	var text strings.Builder
	for index, sample := range samples {
		if index > 0 {
			text.WriteString("\n")
		}
		fmt.Fprintf(&text, "[%s, %s]\n%s\n", sample.Language, sample.SentAt.Format("Jan 2006"), strings.TrimSpace(sample.Content))
	}
	return text.String()
}
