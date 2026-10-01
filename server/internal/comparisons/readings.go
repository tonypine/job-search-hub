package comparisons

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// Reading is one leaf of an answer as the owner judges it: its path, its
// value as text, and the evidence and reason the answer gave beside it.
type Reading struct {
	Field    string `json:"field"`
	Text     string `json:"text"`
	Evidence string `json:"evidence,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// ListReadings returns the answer's leaves in field order, without the
// quoted keys, each with the evidence and reason of the object it sits in.
// An answer that isn't an object, such as a failed one, has none.
func ListReadings(answer json.RawMessage) []Reading {
	readings := []Reading{}
	var reading map[string]any
	if json.Unmarshal(answer, &reading) != nil {
		return readings
	}
	collectReadings("", reading, &readings)
	slices.SortFunc(readings, func(left, right Reading) int { return strings.Compare(left.Field, right.Field) })
	return readings
}

func collectReadings(path string, object map[string]any, readings *[]Reading) {
	evidence, _ := object["evidence"].(string)
	reason, _ := object["reason"].(string)
	for key, value := range object {
		if slices.Contains(quotedKeys, key) {
			continue
		}
		field := key
		if path != "" {
			field = path + "." + key
		}
		if inner, isObject := value.(map[string]any); isObject {
			collectReadings(field, inner, readings)
			continue
		}
		*readings = append(*readings, Reading{Field: field, Text: formatReadingText(value), Evidence: evidence, Reason: reason})
	}
}

// formatReadingText writes a leaf as the owner reads it: text as it is,
// lists of text sorted and joined, null as empty.
func formatReadingText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		if typed {
			return "yes"
		}
		return "no"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []any:
		texts := make([]string, 0, len(typed))
		for _, inner := range typed {
			text, isText := inner.(string)
			if !isText {
				encoded, _ := json.Marshal(typed)
				return string(encoded)
			}
			texts = append(texts, strings.TrimSpace(text))
		}
		slices.Sort(texts)
		return strings.Join(texts, ", ")
	default:
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	}
}
