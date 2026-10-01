// Package comparisons runs one task on the same jobs through several stacks
// and measures how the answers agree, so models can be judged side by side.
package comparisons

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// quotedKeys hold text copied or explained from the posting, not the reading
// itself; two stacks can quote differently and still agree.
var quotedKeys = []string{"evidence", "reason", "as_written"}

// Summary measures each stack against the comparison's first stack, field by
// field, and counts the owner's verdicts. A field is a leaf of the answer,
// named by its path, like location.open_to_brazil: a whole object would
// disagree whenever any free text inside it does.
type Summary struct {
	Fields []string       `json:"fields"`
	Stacks []StackSummary `json:"stacks"`
}

type StackSummary struct {
	StackID  uuid.UUID    `json:"stack_id"`
	Answered int          `json:"answered"`
	Failed   int          `json:"failed"`
	Fields   []FieldScore `json:"fields"`
}

// FieldScore is one field of one stack: on how many jobs it agreed with the
// first stack, out of the jobs both answered, and the owner's verdicts.
type FieldScore struct {
	Field    string `json:"field"`
	Agreed   int    `json:"agreed"`
	Compared int    `json:"compared"`
	Right    int    `json:"right"`
	Wrong    int    `json:"wrong"`
}

type answerKey struct {
	stackID uuid.UUID
	jobID   uuid.UUID
}

// Summarize scores every stack of the record. The first stack has no
// agreement of its own; it is what the others are measured against.
func Summarize(record store.ComparisonRecord) Summary {
	answers := map[answerKey]map[string]any{}
	failed := map[uuid.UUID]int{}
	fieldSet := map[string]bool{}
	for _, answer := range record.Answers {
		if answer.Error != "" || len(answer.Answer) == 0 {
			failed[answer.StackID]++
			continue
		}
		var reading map[string]any
		if json.Unmarshal(answer.Answer, &reading) != nil {
			failed[answer.StackID]++
			continue
		}
		fields := map[string]any{}
		flattenReading("", reading, fields)
		answers[answerKey{answer.StackID, answer.JobID}] = fields
		for field := range fields {
			fieldSet[field] = true
		}
	}
	summary := Summary{Fields: make([]string, 0, len(fieldSet)), Stacks: []StackSummary{}}
	for field := range fieldSet {
		summary.Fields = append(summary.Fields, field)
	}
	slices.Sort(summary.Fields)

	verdicts := map[answerKey]map[string]string{}
	for _, verdict := range record.Verdicts {
		key := answerKey{verdict.StackID, verdict.JobID}
		if verdicts[key] == nil {
			verdicts[key] = map[string]string{}
		}
		verdicts[key][verdict.Field] = verdict.Verdict
	}

	for position, stack := range record.Stacks {
		stackSummary := StackSummary{StackID: stack.ID, Failed: failed[stack.ID], Fields: []FieldScore{}}
		for _, field := range summary.Fields {
			score := FieldScore{Field: field}
			for _, jobID := range record.JobIDs {
				switch verdicts[answerKey{stack.ID, jobID}][field] {
				case "right":
					score.Right++
				case "wrong":
					score.Wrong++
				}
				if position == 0 {
					continue
				}
				reference, hasReference := answers[answerKey{record.Stacks[0].ID, jobID}]
				answer, hasAnswer := answers[answerKey{stack.ID, jobID}]
				if !hasReference || !hasAnswer {
					continue
				}
				score.Compared++
				if reflect.DeepEqual(reference[field], answer[field]) {
					score.Agreed++
				}
			}
			stackSummary.Fields = append(stackSummary.Fields, score)
		}
		for _, jobID := range record.JobIDs {
			if _, has := answers[answerKey{stack.ID, jobID}]; has {
				stackSummary.Answered++
			}
		}
		summary.Stacks = append(summary.Stacks, stackSummary)
	}
	return summary
}

// flattenReading puts each leaf of value into fields under its dotted path,
// normalized, leaving out the quoted keys. A list is one leaf.
func flattenReading(path string, value any, fields map[string]any) {
	object, isObject := value.(map[string]any)
	if !isObject {
		fields[path] = normalizeReading(value)
		return
	}
	for key, inner := range object {
		if slices.Contains(quotedKeys, key) {
			continue
		}
		innerPath := key
		if path != "" {
			innerPath = path + "." + key
		}
		flattenReading(innerPath, inner, fields)
	}
}

// normalizeReading drops the quoted keys, trims and lowercases text, and
// sorts lists of text, so equal readings compare equal however they're written.
func normalizeReading(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := map[string]any{}
		for key, inner := range typed {
			if !slices.Contains(quotedKeys, key) {
				normalized[key] = normalizeReading(inner)
			}
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		texts := make([]string, 0, len(typed))
		for index, inner := range typed {
			normalized[index] = normalizeReading(inner)
			if text, isText := normalized[index].(string); isText {
				texts = append(texts, text)
			}
		}
		if len(texts) == len(typed) {
			slices.Sort(texts)
			return texts
		}
		return normalized
	case string:
		return strings.ToLower(strings.TrimSpace(typed))
	default:
		return typed
	}
}
