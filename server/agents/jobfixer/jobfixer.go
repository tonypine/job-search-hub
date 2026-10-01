// Package jobfixer holds the contract of the agent that corrects a job's
// details from the owner's note: the shape of the result it answers with.
// Its instructions are an editable prompt stored in the hub, not code.
package jobfixer

import _ "embed"

//go:embed result.schema.json
var ResultSchema string

// Result is the job fixer's answer, as ResultSchema describes it.
type Result struct {
	JobID       string   `json:"job_id"`
	FixedFields []string `json:"fixed_fields"`
	Summary     string   `json:"summary"`
}
