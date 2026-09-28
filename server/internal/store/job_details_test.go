package store

import (
	"encoding/json"
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
