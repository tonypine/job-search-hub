package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFactsFollowTheSchemasOrderAndLabels(t *testing.T) {
	schema := json.RawMessage(`{"required":["summary","stack"],"properties":{
		"summary":{"title":"Summary","description":"One line."},
		"stack":{"title":"Technologies"},
		"level":{"title":"Seniority"}}}`)
	facts := json.RawMessage(`{"level":"Senior","stack":["Go"],"summary":"Builds agents.","extra":null}`)

	entries, err := labelJobFacts(facts, schema)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Key+"="+entry.Title)
	}
	want := []string{"summary=Summary", "stack=Technologies", "level=Seniority", "extra=extra"}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("entries = %v, want %v", got, want)
		}
	}
	if entries[0].Description != "One line." || string(entries[1].Value) != `["Go"]` || string(entries[3].Value) != "null" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestFactsReadWithEvidenceAreFlattenedIntoTheirValueAndParts(t *testing.T) {
	schema := json.RawMessage(`{"required":["location","seniority","years_of_experience"],"properties":{
		"location":{"title":"Where they hire","type":"object","required":["evidence","restriction","open_to_brazil","reason"],"properties":{
			"evidence":{"type":"string"},"restriction":{"type":"string"},
			"open_to_brazil":{"title":"Open to Brazil","enum":["yes","no","unclear"]},"reason":{"type":"string"}}},
		"seniority":{"title":"Seniority","type":"object","properties":{"evidence":{"type":"string"},"as_written":{"type":"string"},"levels":{"type":"array"}}},
		"years_of_experience":{"title":"Years","type":"object","properties":{"evidence":{"type":"string"},"value":{}}}}}`)
	facts := json.RawMessage(`{
		"location":{"evidence":"**Remote** within LATAM","restriction":"LATAM","open_to_brazil":"yes","reason":"LATAM contains Brazil."},
		"seniority":{"evidence":"### ","as_written":"Senior","levels":["senior"]},
		"years_of_experience":{"evidence":"  - 5+ years","value":5}}`)

	entries, err := labelJobFacts(facts, schema)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Key+"="+entry.Title+"="+string(entry.Value)+"="+entry.Evidence)
	}
	want := []string{
		`location=Where they hire="LATAM"=Remote within LATAM`, `location.open_to_brazil=Open to Brazil="yes"=`, `location.reason=Reason="LATAM contains Brazil."=`,
		`seniority=Seniority="Senior"=`, `seniority.levels=Levels=["senior"]=`, `years_of_experience=Years=5=5+ years`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("entries:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	flat := FlattenJobFactsToJSON(facts)
	if string(FlattenJobFactsToJSON(flat)) != string(flat) || !strings.Contains(string(flat), `"location.open_to_brazil":"yes"`) {
		t.Fatalf("flattened = %s; want a flat object that flattens to itself", flat)
	}
	labels, _ := listJobFactLabels(schema)
	if len(labels) != 6 || labels[1].Key != "location.open_to_brazil" {
		t.Fatalf("columns = %+v", labels)
	}
}
