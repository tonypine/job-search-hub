package store

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// A fact read with its evidence is an object: the quote it rests on under
// "evidence", its value under one of jobFactValueParts, and sometimes more
// parts, such as a location's "open_to_brazil" and "reason". Facts read under
// an older schema are plain values.
var jobFactValueParts = []string{"value", "as_written", "restriction"}

const jobFactEvidencePart = "evidence"

// FlattenJobFacts turns each fact that's an object with evidence into its
// value, its evidence, and one fact per other part, keyed "<fact>.<part>".
// Plain facts pass through, so flattening flat facts changes nothing.
func FlattenJobFacts(raw json.RawMessage) (values map[string]json.RawMessage, evidence map[string]string, err error) {
	var facts map[string]json.RawMessage
	if err := json.Unmarshal(raw, &facts); err != nil {
		return nil, nil, err
	}
	values, evidence = map[string]json.RawMessage{}, map[string]string{}
	for key, fact := range facts {
		var parts map[string]json.RawMessage
		if json.Unmarshal(fact, &parts) != nil || !isJobFactWithParts(parts) {
			values[key] = fact
			continue
		}
		valuePart := getJobFactValuePart(parts)
		values[key] = parts[valuePart]
		if quoteJSON, has := parts[jobFactEvidencePart]; has {
			var quote string
			if json.Unmarshal(quoteJSON, &quote) == nil && strings.TrimSpace(quote) != "" {
				evidence[key] = quote
			}
		}
		for part, partValue := range parts {
			if part != valuePart && part != jobFactEvidencePart {
				values[key+"."+part] = partValue
			}
		}
	}
	return values, evidence, nil
}

// FlattenJobFactsToJSON is FlattenJobFacts' values as one JSON object, as
// the jobs list and the fit checks read them. Facts that aren't a JSON
// object come back unchanged.
func FlattenJobFactsToJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	values, _, err := FlattenJobFacts(raw)
	if err != nil {
		return raw
	}
	flat, err := json.Marshal(values)
	if err != nil {
		return raw
	}
	return flat
}

func isJobFactWithParts(parts map[string]json.RawMessage) bool {
	if parts == nil {
		return false
	}
	_, hasEvidence := parts[jobFactEvidencePart]
	return hasEvidence || getJobFactValuePart(parts) != ""
}

func getJobFactValuePart(parts map[string]json.RawMessage) string {
	for _, name := range jobFactValueParts {
		if _, has := parts[name]; has {
			return name
		}
	}
	return ""
}

// jobFactLabel is a flat fact's key with the schema's title and description
// for it.
type jobFactLabel struct {
	Key         string
	Title       string
	Description string
}

type jobFactSchemaProperty struct {
	Title       string                           `json:"title"`
	Description string                           `json:"description"`
	Required    []string                         `json:"required"`
	Properties  map[string]jobFactSchemaProperty `json:"properties"`
}

// listJobFactLabels labels every flat fact the schema describes, in the
// schema's order (its required list, then its other properties): each fact,
// then the parts of a fact with evidence other than its value and evidence.
// A fact without a title is labelled by its key, and a part by its name.
func listJobFactLabels(schema json.RawMessage) ([]jobFactLabel, error) {
	var root jobFactSchemaProperty
	if len(schema) > 0 {
		if err := json.Unmarshal(schema, &root); err != nil {
			return nil, err
		}
	}
	var labels []jobFactLabel
	for _, key := range getOrderedPropertyNames(root) {
		property := root.Properties[key]
		title := property.Title
		if title == "" {
			title = key
		}
		labels = append(labels, jobFactLabel{Key: key, Title: title, Description: property.Description})
		valuePart := ""
		for _, name := range jobFactValueParts {
			if _, has := property.Properties[name]; has {
				valuePart = name
				break
			}
		}
		for _, part := range getOrderedPropertyNames(property) {
			if part == valuePart || part == jobFactEvidencePart {
				continue
			}
			partProperty := property.Properties[part]
			labels = append(labels, jobFactLabel{Key: key + "." + part, Title: getPartTitle(partProperty, part), Description: partProperty.Description})
		}
	}
	return labels, nil
}

func getOrderedPropertyNames(property jobFactSchemaProperty) []string {
	var names []string
	seen := map[string]bool{}
	for _, name := range append(slices.Clone(property.Required), slices.Sorted(maps.Keys(property.Properties))...) {
		if _, known := property.Properties[name]; known && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func getPartTitle(property jobFactSchemaProperty, name string) string {
	if property.Title != "" {
		return property.Title
	}
	words := strings.ReplaceAll(name, "_", " ")
	return strings.ToUpper(words[:1]) + words[1:]
}
