// Package wordmatch finds words in text the way people read them: whole
// words, without case or accents.
package wordmatch

import "strings"

var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e", "í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o", "ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
)

// Normalize lowers the case of text and drops its accents, so "Sênior"
// becomes "senior".
func Normalize(text string) string {
	return accents.Replace(strings.ToLower(strings.TrimSpace(text)))
}

// Contains reports whether normalized text holds the normalized words as
// whole words: "us only" is not in "focus only".
func Contains(text, words string) bool {
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
