package companytriage_test

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/tonypine/job-search-hub/server/agents/companytriage"
)

func resolveResultSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	var schema jsonschema.Schema
	if err := json.Unmarshal([]byte(companytriage.ResultSchema), &schema); err != nil {
		t.Fatalf("parse the result schema: %v", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve the result schema: %v", err)
	}
	return resolved
}

func decodeInstance(t *testing.T, raw string) any {
	t.Helper()
	var instance any
	if err := json.Unmarshal([]byte(raw), &instance); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return instance
}

func TestTheResultSchemaAcceptsACompleteResult(t *testing.T) {
	resolved := resolveResultSchema(t)
	for _, raw := range []string{
		`{"company_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","domain":"acme.com",
		  "job_board":{"provider":"greenhouse","board_token":"acme","verified":true},
		  "people_added":2,"unresolved":[],"summary":"Filled the dossier."}`,
		`{"company_id":null,"domain":"","job_board":null,"people_added":0,
		  "unresolved":["could not identify the company"],"summary":"Stopped early."}`,
	} {
		if err := resolved.Validate(decodeInstance(t, raw)); err != nil {
			t.Errorf("valid result rejected: %v\n%s", err, raw)
		}
	}
}

func TestTheResultSchemaRejectsAnIncompleteResult(t *testing.T) {
	resolved := resolveResultSchema(t)
	for _, raw := range []string{
		`{"company_id":null,"domain":"acme.com","job_board":null,"people_added":0,"unresolved":[]}`,
		`{"company_id":null,"domain":"acme.com","job_board":null,"people_added":-1,"unresolved":[],"summary":""}`,
	} {
		if err := resolved.Validate(decodeInstance(t, raw)); err == nil {
			t.Errorf("incomplete result accepted: %s", raw)
		}
	}
}

func TestTheResultTypeMatchesTheSchema(t *testing.T) {
	raw := `{"company_id":"id-1","domain":"acme.com","job_board":{"provider":"lever","board_token":"acme","verified":true},
	         "people_added":1,"unresolved":["no headquarters on the site"],"summary":"Done."}`
	var result companytriage.Result
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if *result.CompanyID != "id-1" || result.JobBoard.BoardToken != "acme" || result.PeopleAdded != 1 || len(result.Unresolved) != 1 {
		t.Fatalf("result = %+v", result)
	}
}
